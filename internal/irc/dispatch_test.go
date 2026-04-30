package irc

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"euphrates/internal/state"
)

// recorder collects every callback fire for assertions.
type recorder struct {
	self     string
	messages []state.Message
	events   []string
	joins    []string
	parts    []string
	lists    [][]string
}

var timestampPrefixPattern = regexp.MustCompile(`^\d\d:\d\d:\d\d\s`)

func newRecorder(self string) (*recorder, Handlers) {
	r := &recorder{self: self}
	return r, Handlers{
		Self:      func() string { return r.self },
		OnMessage: func(m state.Message) { r.messages = append(r.messages, m) },
		OnEvent:   func(s string) { r.events = append(r.events, s) },
		OnJoin:    func(c string) { r.joins = append(r.joins, c) },
		OnPart:    func(c string) { r.parts = append(r.parts, c) },
		OnChannelList: func(names []string) {
			cp := make([]string, len(names))
			copy(cp, names)
			r.lists = append(r.lists, cp)
		},
	}
}

func assertHasTimestampPrefix(t *testing.T, line string) {
	t.Helper()
	if !timestampPrefixPattern.MatchString(line) {
		t.Fatalf("missing hh:mm:ss prefix: %q", line)
	}
}

func assertRecentMessageTime(t *testing.T, got time.Time) {
	t.Helper()
	if got.IsZero() {
		t.Fatal("message time is zero")
	}
	now := time.Now()
	if got.Before(now.Add(-2*time.Second)) || got.After(now.Add(2*time.Second)) {
		t.Fatalf("message time %v out of expected range around %v", got, now)
	}
}

// --- nickFromSource -------------------------------------------------------

func TestNickFromSource(t *testing.T) {
	cases := map[string]string{
		"alice!u@host":    "alice",
		"alice":           "alice",
		"irc.libera.chat": "",
		"":                "",
	}
	for in, want := range cases {
		if got := nickFromSource(in); got != want {
			t.Errorf("nickFromSource(%q)=%q want %q", in, got, want)
		}
	}
}

// --- dispatchPrivmsg ------------------------------------------------------

func TestDispatchPrivmsg_ChannelMessage(t *testing.T) {
	r, h := newRecorder("me")
	dispatchPrivmsg(h, "alice!u@host", "#foo", "hello", state.KindPrivmsg)
	if len(r.messages) != 1 {
		t.Fatalf("messages=%v", r.messages)
	}
	got := r.messages[0]
	want := state.Message{Channel: "#foo", Nick: "alice", Text: "hello", Kind: state.KindPrivmsg}
	assertRecentMessageTime(t, got.Time)
	got.Time = time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%+v want=%+v", got, want)
	}
}

func TestDispatchPrivmsg_DirectMessage(t *testing.T) {
	r, h := newRecorder("me")
	dispatchPrivmsg(h, "alice!u@host", "me", "hi", state.KindPrivmsg)
	if len(r.messages) != 1 || r.messages[0].Channel != "alice" {
		t.Errorf("expected query keyed by sender; got %+v", r.messages)
	}
	assertRecentMessageTime(t, r.messages[0].Time)
}

func TestDispatchPrivmsg_DirectMessageCaseInsensitive(t *testing.T) {
	r, h := newRecorder("Me")
	dispatchPrivmsg(h, "alice!u@host", "ME", "hi", state.KindPrivmsg)
	if len(r.messages) != 1 || r.messages[0].Channel != "alice" {
		t.Errorf("case-insensitive self match failed: %+v", r.messages)
	}
	assertRecentMessageTime(t, r.messages[0].Time)
}

func TestDispatchPrivmsg_Action(t *testing.T) {
	r, h := newRecorder("me")
	dispatchPrivmsg(h, "alice!u@host", "#foo", "waves", state.KindAction)
	if len(r.messages) != 1 || r.messages[0].Kind != state.KindAction {
		t.Errorf("action not routed: %+v", r.messages)
	}
	assertRecentMessageTime(t, r.messages[0].Time)
}

func TestDispatchPrivmsg_DropsServerSource(t *testing.T) {
	r, h := newRecorder("me")
	dispatchPrivmsg(h, "irc.libera.chat", "#foo", "hi", state.KindPrivmsg)
	if len(r.messages) != 0 {
		t.Errorf("server-sourced PRIVMSG leaked: %+v", r.messages)
	}
}

// --- dispatchNotice -------------------------------------------------------

func TestDispatchNotice_ChannelRoutesAsMessage(t *testing.T) {
	r, h := newRecorder("me")
	dispatchNotice(h, "alice!u@host", "#foo", "fyi")
	if len(r.messages) != 1 || r.messages[0].Channel != "#foo" || r.messages[0].Kind != state.KindNotice {
		t.Errorf("channel notice mis-routed: %+v", r.messages)
	}
	assertRecentMessageTime(t, r.messages[0].Time)
}

func TestDispatchNotice_ToSelfRoutesToServer(t *testing.T) {
	r, h := newRecorder("me")
	dispatchNotice(h, "alice!u@host", "me", "ping")
	if len(r.messages) != 1 || r.messages[0].Channel != state.ServerChannelName {
		t.Errorf("self-notice should land in server log: %+v", r.messages)
	}
	assertRecentMessageTime(t, r.messages[0].Time)
}

func TestDispatchNotice_PreRegistrationStarTarget(t *testing.T) {
	r, h := newRecorder("")
	dispatchNotice(h, "irc.libera.chat", "*", "*** Looking up your hostname")
	if len(r.messages) != 1 || r.messages[0].Channel != state.ServerChannelName {
		t.Errorf("pre-reg notice to '*' should land in server log: %+v", r.messages)
	}
	assertRecentMessageTime(t, r.messages[0].Time)
}

// --- dispatchJoin / dispatchPart -----------------------------------------

func TestDispatchJoin_Self(t *testing.T) {
	r, h := newRecorder("me")
	dispatchJoin(h, "me!u@host", "#foo")
	if !reflect.DeepEqual(r.joins, []string{"#foo"}) {
		t.Errorf("self-join not routed: %+v", r.joins)
	}
	if len(r.events) == 0 || !strings.Contains(r.events[0], "joined #foo") {
		t.Errorf("missing join event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchJoin_Other(t *testing.T) {
	r, h := newRecorder("me")
	dispatchJoin(h, "alice!u@host", "#foo")
	if len(r.joins) != 0 {
		t.Errorf("other join leaked into OnJoin: %+v", r.joins)
	}
	if len(r.events) != 1 || !strings.Contains(r.events[0], "alice") {
		t.Errorf("expected alice event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchPart_SelfAndReason(t *testing.T) {
	r, h := newRecorder("me")
	dispatchPart(h, "me!u@host", "#foo", "bye")
	if !reflect.DeepEqual(r.parts, []string{"#foo"}) {
		t.Errorf("self-part not routed: %+v", r.parts)
	}
	if len(r.events) != 1 || !strings.Contains(r.events[0], "(bye)") {
		t.Errorf("reason missing: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchPart_Other(t *testing.T) {
	r, h := newRecorder("me")
	dispatchPart(h, "alice!u@host", "#foo", "")
	if len(r.parts) != 0 {
		t.Errorf("other part leaked: %+v", r.parts)
	}
	if len(r.events) != 1 || !strings.Contains(r.events[0], "alice") {
		t.Errorf("expected event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

// --- dispatchQuit / dispatchNick / dispatchTopic / dispatchKick ----------

func TestDispatchQuit(t *testing.T) {
	r, h := newRecorder("me")
	dispatchQuit(h, "alice!u@host", "ping timeout")
	if len(r.events) != 1 || !strings.Contains(r.events[0], "alice quit") {
		t.Errorf("quit event: %+v", r.events)
	}
	if !strings.Contains(r.events[0], "(ping timeout)") {
		t.Errorf("reason missing: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchNick_SelfRename(t *testing.T) {
	r, h := newRecorder("me")
	dispatchNick(h, "me!u@host", "newme")
	if len(r.events) != 1 || !strings.Contains(r.events[0], "you are now newme") {
		t.Errorf("self-nick event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchNick_OtherRename(t *testing.T) {
	r, h := newRecorder("me")
	dispatchNick(h, "alice!u@host", "alicia")
	if len(r.events) != 1 || !strings.Contains(r.events[0], "alice is now alicia") {
		t.Errorf("nick event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchTopic(t *testing.T) {
	r, h := newRecorder("me")
	dispatchTopic(h, "alice!u@host", "#foo", "be excellent")
	if len(r.events) != 1 || !strings.Contains(r.events[0], "be excellent") {
		t.Errorf("topic event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchKick_Self(t *testing.T) {
	r, h := newRecorder("me")
	dispatchKick(h, "op!u@host", "#foo", "me", "shoo")
	if !reflect.DeepEqual(r.parts, []string{"#foo"}) {
		t.Errorf("self-kick should drop channel: %+v", r.parts)
	}
	if len(r.events) != 1 || !strings.Contains(r.events[0], "kicked from #foo") {
		t.Errorf("kick event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

func TestDispatchKick_Other(t *testing.T) {
	r, h := newRecorder("me")
	dispatchKick(h, "op!u@host", "#foo", "alice", "rude")
	if len(r.parts) != 0 {
		t.Errorf("kick of other should not part: %+v", r.parts)
	}
	if len(r.events) != 1 || !strings.Contains(r.events[0], "kicked alice") {
		t.Errorf("kick event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

// --- dispatchMode ---------------------------------------------------------

func TestDispatchMode(t *testing.T) {
	r, h := newRecorder("me")
	dispatchMode(h, "op!u@host", "#foo", []string{"+o", "alice"})
	if len(r.events) != 1 || !strings.Contains(r.events[0], "+o alice") {
		t.Errorf("mode event: %+v", r.events)
	}
	assertHasTimestampPrefix(t, r.events[0])
}

// --- dispatchServerNumeric -----------------------------------------------

func TestDispatchServerNumeric_DropsLeadingNick(t *testing.T) {
	r, h := newRecorder("me")
	dispatchServerNumeric(h, "372", []string{"me", "- welcome to libera -"})
	if len(r.messages) != 1 {
		t.Fatalf("messages=%+v", r.messages)
	}
	got := r.messages[0]
	if got.Channel != state.ServerChannelName {
		t.Errorf("channel=%q", got.Channel)
	}
	if got.Nick != "372" {
		t.Errorf("nick=%q", got.Nick)
	}
	if got.Text != "- welcome to libera -" {
		t.Errorf("text=%q", got.Text)
	}
	assertRecentMessageTime(t, got.Time)
}

func TestDispatchServerNumeric_EmptyParamsIsNoop(t *testing.T) {
	r, h := newRecorder("me")
	dispatchServerNumeric(h, "001", nil)
	if len(r.messages) != 0 {
		t.Errorf("expected no message: %+v", r.messages)
	}
}

// --- nil-handler safety ---------------------------------------------------

func TestNilHandlersDoNotPanic(t *testing.T) {
	h := Handlers{}
	dispatchPrivmsg(h, "alice!u@host", "#foo", "x", state.KindPrivmsg)
	dispatchJoin(h, "alice!u@host", "#foo")
	dispatchPart(h, "alice!u@host", "#foo", "")
	dispatchQuit(h, "alice!u@host", "")
	dispatchNick(h, "alice!u@host", "alicia")
	dispatchTopic(h, "alice!u@host", "#foo", "x")
	dispatchKick(h, "op!u@host", "#foo", "alice", "")
	dispatchMode(h, "op", "#foo", []string{"+o"})
	dispatchServerNumeric(h, "001", []string{"me", "welcome"})
	dispatchNotice(h, "x!u@h", "#foo", "y")
	h.emitChannelList([]string{"#a"})
}

func TestEmitChannelList(t *testing.T) {
	r, h := newRecorder("me")
	h.emitChannelList([]string{"#a", "#b"})
	if len(r.lists) != 1 {
		t.Fatalf("lists=%v", r.lists)
	}
	if !reflect.DeepEqual(r.lists[0], []string{"#a", "#b"}) {
		t.Fatalf("list=%v", r.lists[0])
	}
}

// --- hostOnly -------------------------------------------------------------

func TestHostOnly(t *testing.T) {
	cases := map[string]string{
		"irc.libera.chat:6697": "irc.libera.chat",
		"irc.libera.chat":      "irc.libera.chat",
		"":                     "",
	}
	for in, want := range cases {
		if got := hostOnly(in); got != want {
			t.Errorf("hostOnly(%q)=%q want %q", in, got, want)
		}
	}
}
