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
	// Check for group headers with horizontal lines and channels
	for _, digit := range []string{"1", "2", "0"} {
		found := false
		for _, line := range strings.Split(got, "\n") {
			if strings.Contains(line, digit) && strings.Contains(line, "─") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("sidebar missing group header for %q in %q", digit, got)
		}
	}
	if !strings.Contains(got, "  #a") || !strings.Contains(got, "  #k") {
		t.Fatalf("sidebar missing channels in %q", got)
	}
	// Verify group 1 has channels in insertion order
	lines := strings.Split(got, "\n")
	foundGroup1 := false
	for i, line := range lines {
		if strings.Contains(line, "1") && strings.Contains(line, "─") {
			foundGroup1 = true
			// Next two lines should be #a and #k
			if i+2 < len(lines) && lines[i+1] == "  #a" && lines[i+2] == "  #k" {
				break
			}
			t.Fatalf("group 1 ordering wrong:\n%s", got)
		}
	}
	if !foundGroup1 {
		t.Fatalf("group 1 header not found:\n%s", got)
	}
}
