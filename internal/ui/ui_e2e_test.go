package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"euphrates/internal/state"
)

type uiGroupingStrategyFunc func(state.GroupingInput) (state.Assignment, bool, error)

func (f uiGroupingStrategyFunc) Apply(input state.GroupingInput) (state.Assignment, bool, error) {
	return f(input)
}

func TestJoinCompletionFlow_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)

	// Simulate an existing joined channel and a prefetched LIST cache.
	u.state.JoinChannel("#go")
	u.state.SetChannelListCache([]string{"#go", "#golang", "#gophers"})

	// First tab on an empty /join argument inserts '#'.
	u.input.SetText("/join ")
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("tab not consumed at /join prompt")
	}
	if got := u.input.GetText(); got != "/join #" {
		t.Fatalf("input=%q want /join #", got)
	}

	// Tab-complete using prefetched LIST and exclusion of already joined #go.
	u.input.SetText("/join #gola")
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("tab not consumed at partial")
	}
	if got := u.input.GetText(); got != "/join #golang" {
		t.Fatalf("input=%q want /join #golang", got)
	}

	// Submit /join command end-to-end through input handler.
	u.onInputDone(tcell.KeyEnter)
	if len(fs.joins) != 1 || fs.joins[0] != "#golang" {
		t.Fatalf("joins=%v", fs.joins)
	}
}

func TestJoinCompletion_DoubleTabAfterLCPExpansion_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)
	u.eventsView.SetRect(0, 0, 120, 5)
	u.state.SetChannelListCache([]string{
		"#archive",
		"#archivebot-alerts",
		"#archivebot-bs",
		"#archiveteam-internal",
		"#archiveteam-matrix",
	})

	u.input.SetText("/join #arch")
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("first tab not consumed")
	}
	if got := u.input.GetText(); got != "/join #archive" {
		t.Fatalf("input=%q want /join #archive", got)
	}

	before := len(u.state.Events())
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("second tab not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected completion suggestions in events, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	for _, want := range []string{"#archive", "#archivebot-alerts", "#archivebot-bs", "#archiveteam-internal", "#archiveteam-matrix"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in events: %v", want, after[before:])
		}
	}
}

func TestPartCompletion_DoubleTabAfterLCPExpansion_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)
	u.eventsView.SetRect(0, 0, 120, 5)
	u.state.JoinChannel("#archive")
	u.state.JoinChannel("#archivebot-alerts")
	u.state.JoinChannel("#archivebot-bs")
	u.state.JoinChannel("#archiveteam-internal")
	u.state.JoinChannel("#archiveteam-matrix")

	u.input.SetText("/part #arch")
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("first tab not consumed")
	}
	if got := u.input.GetText(); got != "/part #archive" {
		t.Fatalf("input=%q want /part #archive", got)
	}

	before := len(u.state.Events())
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("second tab not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected completion suggestions in events, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	for _, want := range []string{"#archive", "#archivebot-alerts", "#archivebot-bs", "#archiveteam-internal", "#archiveteam-matrix"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in events: %v", want, after[before:])
		}
	}
}

func TestCommandCompletionAndHelp_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)
	u.eventsView.SetRect(0, 0, 120, 5)

	u.input.SetText("/h")
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("tab not consumed for /h")
	}
	if got := u.input.GetText(); got != "/help " {
		t.Fatalf("input=%q want /help ", got)
	}

	u.input.SetText("/")
	before := len(u.state.Events())
	if got := u.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)); got != nil {
		t.Fatalf("tab not consumed for ambiguous /")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected command completion suggestions, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	for _, want := range []string{"/help", "/join", "/list", "/me", "/part", "/quit"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in events: %v", want, after[before:])
		}
	}

	u.input.SetText("/help")
	u.onInputDone(tcell.KeyEnter)
	ev := u.state.Events()
	if len(ev) == 0 {
		t.Fatal("expected help output in events")
	}
	joinedEvents := strings.Join(ev, " ")
	for _, want := range []string{"commands:", "/help", "/join", "/quit", "usage:"} {
		if !strings.Contains(joinedEvents, want) {
			t.Fatalf("missing %q in help output: %v", want, ev)
		}
	}
}

func TestChannelBrowser_OpenFilterJoin_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)

	u.state.SetChannelListCache([]string{"#archive", "#beta", "#gamma"})
	u.state.JoinChannel("#beta")

	if got := u.handleKey(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModAlt)); got != nil {
		t.Fatalf("Alt+L not consumed")
	}
	if !u.browserVisible {
		t.Fatal("browser should be visible")
	}

	if got := u.handleKey(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone)); got != nil {
		t.Fatalf("typed filter not consumed")
	}
	if got := u.browser.searchQuery; got != "a" {
		t.Fatalf("query=%q want a", got)
	}
	if got := u.browser.selectedChannel(); got != "#archive" {
		t.Fatalf("selected=%q want #archive", got)
	}

	if got := u.handleKey(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModAlt)); got != nil {
		t.Fatalf("Alt+J not consumed")
	}
	if len(fs.joins) != 1 || fs.joins[0] != "#archive" {
		t.Fatalf("joins=%v", fs.joins)
	}
	if u.browserVisible {
		t.Fatal("browser should close after Alt+J")
	}
}

func TestJoinAndPartHooks_ApplyRegroupingStrategy_E2E(t *testing.T) {
	strategy := uiGroupingStrategyFunc(func(input state.GroupingInput) (state.Assignment, bool, error) {
		out := make(state.Assignment, len(input.Channels))
		group := state.GroupID(0)
		if input.Trigger == state.GroupingTriggerPart {
			group = 7
		}
		for _, ch := range input.Channels {
			out[ch.Name] = group
		}
		return out, true, nil
	})

	s := state.New(state.Config{MessageCap: 100, EventCap: 20, Grouping: strategy})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)

	s.JoinChannel("#a")
	s.JoinChannel("#b")
	s.AppendMessage(state.Message{Channel: "#a", Nick: "alice", Text: "a1", Kind: state.KindPrivmsg})
	s.AppendMessage(state.Message{Channel: "#b", Nick: "bob", Text: "b1", Kind: state.KindPrivmsg})
	u.refreshMain()

	s.PartChannel("#a")
	u.refreshAfterStructuralChange()
	b, ok := s.Channel("#b")
	if !ok {
		t.Fatal("#b channel missing after part")
	}
	if b.Group != 7 {
		t.Fatalf("#b group=%d want 7", b.Group)
	}
	s.SetVisible(state.GroupID(7), false)
	u.refreshMain()

	if lines := s.RenderVisible(); len(lines) != 0 {
		t.Fatalf("expected no visible lines after hiding regrouped channel, got %d", len(lines))
	}
}

func TestSidebarToggleAndJoinUpdates_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)

	if got := u.handleKey(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModAlt)); got != nil {
		t.Fatalf("Alt+g not consumed")
	}
	if !u.sidebarVisible {
		t.Fatal("sidebar should be visible")
	}

	u.state.JoinChannel("#a")
	u.state.JoinChannel("#b")
	u.state.JoinChannel("#c")
	u.state.JoinChannel("#d")
	u.state.JoinChannel("#e")
	u.state.JoinChannel("#f")
	u.state.JoinChannel("#g")
	u.state.JoinChannel("#h")
	u.state.JoinChannel("#i")
	u.state.JoinChannel("#j")
	u.state.JoinChannel("#k")
	u.refreshSidebar()

	got := u.sidebarView.GetText(true)
	// Check for group headers with visibility indicators, lines, and channels.
	for _, digit := range []string{"1", "2", "0"} {
		found := false
		for _, line := range strings.Split(got, "\n") {
			if strings.HasPrefix(line, "● ") && strings.Contains(line, " "+digit+" ") && strings.Contains(line, "─") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("sidebar missing group header for %q in %q", digit, got)
		}
	}

	u.state.SetTarget("#b")
	u.state.SetVisible(state.GroupID(1), false)
	u.refreshSidebar()
	hidden := u.sidebarView.GetText(true)
	if !strings.Contains(hidden, "\n○ 2 ") && !strings.HasPrefix(hidden, "○ 2 ") {
		t.Fatalf("group 2 hidden indicator missing in %q", hidden)
	}
	if !strings.Contains(hidden, "\n▶ #b") && !strings.HasPrefix(hidden, "▶ #b") {
		t.Fatalf("active-target marker missing for #b in hidden group: %q", hidden)
	}

	if !strings.Contains(got, "▶ #a") || !strings.Contains(got, "  #k") {
		t.Fatalf("sidebar missing channels in %q", got)
	}
	// Verify group 1 has channels in insertion order
	lines := strings.Split(got, "\n")
	foundGroup1 := false
	for i, line := range lines {
		if strings.HasPrefix(line, "● 1 ") && strings.Contains(line, "─") {
			foundGroup1 = true
			// Next two lines should be #a and #k; #a is initial target.
			if i+2 < len(lines) && lines[i+1] == "▶ #a" && lines[i+2] == "  #k" {
				break
			}
			t.Fatalf("group 1 ordering wrong:\n%s", got)
		}
	}
	if !foundGroup1 {
		t.Fatalf("group 1 header not found:\n%s", got)
	}
}

func TestUsersPanelToggleAndLiveUpdates_E2E(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 20})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)

	if got := u.handleKey(tcell.NewEventKey(tcell.KeyRune, 'u', tcell.ModAlt)); got != nil {
		t.Fatalf("Alt+u not consumed")
	}
	if !u.usersVisible {
		t.Fatal("users panel should be visible")
	}

	u.state.JoinChannel("#a")
	u.state.SetChannelUsers("#a", []string{"alice", "bob"})
	u.state.AppendMessage(state.Message{Channel: "#a", Nick: "bob", Text: "hi", Kind: state.KindPrivmsg})
	u.state.SetTarget("#a")
	u.refreshUsersPanel()

	title := u.usersTitleView.GetText(true)
	if !strings.Contains(title, "Users — 2") {
		t.Fatalf("users title=%q want count 2", title)
	}
	body := u.usersView.GetText(true)
	if !strings.Contains(body, "bob") || !strings.Contains(body, "alice") {
		t.Fatalf("users panel body missing users: %q", body)
	}
	if !strings.Contains(body, "•") {
		t.Fatalf("users panel missing active-channel indicator: %q", body)
	}

	if got := u.handleKey(tcell.NewEventKey(tcell.KeyRune, 'U', tcell.ModAlt)); got != nil {
		t.Fatalf("Alt+U not consumed")
	}
	if u.usersVisible {
		t.Fatal("users panel should be hidden after second toggle")
	}
}
