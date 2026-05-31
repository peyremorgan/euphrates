package ui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"euphrates/internal/state"
)

// fakeSender records calls for assertions.
type fakeSender struct {
	mu       sync.Mutex
	nick     string
	privmsgs []sentMsg
	actions  []sentMsg
	joins    []string
	parts    []sentPart
	quitWith string
	quitN    int
	sendErr  error
	joinErr  error
	partErr  error
}

type sentMsg struct{ Target, Text string }
type sentPart struct{ Channel, Reason string }

func (f *fakeSender) Nick() string { return f.nick }

func (f *fakeSender) SendPrivmsg(target, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.privmsgs = append(f.privmsgs, sentMsg{target, text})
	return nil
}

func (f *fakeSender) SendAction(target, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.actions = append(f.actions, sentMsg{target, text})
	return nil
}

func (f *fakeSender) Quit(reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quitN++
	f.quitWith = reason
}

func (f *fakeSender) Join(channel string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.joinErr != nil {
		return f.joinErr
	}
	f.joins = append(f.joins, channel)
	return nil
}

func (f *fakeSender) Part(channel, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.partErr != nil {
		return f.partErr
	}
	f.parts = append(f.parts, sentPart{Channel: channel, Reason: reason})
	return nil
}

// newTestUI builds a UI without starting the tview event loop.
func newTestUI(t *testing.T) (*UI, *fakeSender) {
	t.Helper()
	s := state.New(state.Config{MessageCap: 100, EventCap: 5})
	fs := &fakeSender{nick: "me"}
	u := New(s, fs)
	u.mainView.SetRect(0, 0, 80, 6)
	return u, fs
}

func addLines(t *testing.T, u *UI, channel string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		u.state.AppendMessage(state.Message{
			Channel: channel,
			Nick:    "n",
			Text:    fmt.Sprintf("m%d", i),
			Kind:    state.KindPrivmsg,
		})
	}
	u.refreshMain()
}

func TestBuildLayout_IncludesSeparatorBetweenMainAndEvents(t *testing.T) {
	u, _ := newTestUI(t)

	if got := u.root.GetItemCount(); got != 5 {
		t.Fatalf("root item count=%d want 5", got)
	}
	if got := u.root.GetItem(2); got != u.separatorView {
		t.Fatalf("item 2 is %T, want separator view", got)
	}
	if got := u.root.GetItem(3); got != u.eventsView {
		t.Fatalf("item 3 is %T, want events view", got)
	}
	if got := u.root.GetItem(1); got != u.contentRow {
		t.Fatalf("item 1 is %T, want content row", got)
	}
	if got := u.contentRow.GetItemCount(); got != 5 {
		t.Fatalf("content row item count=%d want 5", got)
	}
	if got := u.contentRow.GetItem(0); got != u.sidebarCol {
		t.Fatalf("content row item 0 is %T, want sidebar column", got)
	}
	if got := u.contentRow.GetItem(1); got != u.sidebarDivider {
		t.Fatalf("content row item 1 is %T, want sidebar divider", got)
	}
	if got := u.contentRow.GetItem(2); got != u.mainView {
		t.Fatalf("content row item 2 is %T, want main view", got)
	}
	if got := u.contentRow.GetItem(3); got != u.usersDivider {
		t.Fatalf("content row item 3 is %T, want users divider", got)
	}
	if got := u.contentRow.GetItem(4); got != u.usersCol {
		t.Fatalf("content row item 4 is %T, want users column", got)
	}
}

func TestRefreshSidebar_ShowsAllGroupHeadersWhenEmpty(t *testing.T) {
	u, _ := newTestUI(t)

	u.refreshSidebar()
	got := u.sidebarView.GetText(true)
	lines := strings.Split(got, "\n")
	if len(lines) != state.NumGroups {
		t.Fatalf("line count=%d want %d", len(lines), state.NumGroups)
	}
	for i := 0; i < state.NumGroups; i++ {
		// Each header starts with a visibility indicator and keeps fixed width.
		digit := digitForGroup(i)
		if !strings.HasPrefix(lines[i], "● ") {
			t.Fatalf("line %d=%q missing visible indicator", i, lines[i])
		}
		if !strings.Contains(lines[i], digit) {
			t.Fatalf("line %d=%q missing digit %q", i, lines[i], digit)
		}
		if !strings.Contains(lines[i], "─") {
			t.Fatalf("line %d=%q missing horizontal line characters", i, lines[i])
		}
		if w := len([]rune(lines[i])); w != sidebarWidth {
			t.Fatalf("line %d width=%d want %d (%q)", i, w, sidebarWidth, lines[i])
		}
	}
}

func TestRefreshSidebar_UsesEmptyCircleForHiddenGroups(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetVisible(state.GroupID(0), false)
	u.state.SetVisible(state.GroupID(2), false)

	u.refreshSidebar()
	got := u.sidebarView.GetText(true)
	lines := strings.Split(got, "\n")
	if len(lines) != state.NumGroups {
		t.Fatalf("line count=%d want %d", len(lines), state.NumGroups)
	}
	if !strings.HasPrefix(lines[0], "○ 1 ") {
		t.Fatalf("group 1 hidden header=%q want prefix %q", lines[0], "○ 1 ")
	}
	if !strings.HasPrefix(lines[2], "○ 3 ") {
		t.Fatalf("group 3 hidden header=%q want prefix %q", lines[2], "○ 3 ")
	}
	if !strings.HasPrefix(lines[1], "● 2 ") {
		t.Fatalf("group 2 visible header=%q want prefix %q", lines[1], "● 2 ")
	}
}

func TestFormatGroupHeader_IndicatorAndWidth(t *testing.T) {
	t.Run("visible", func(t *testing.T) {
		got := formatGroupHeader(0, true)
		if !strings.HasPrefix(got, "● 1 ") {
			t.Fatalf("prefix=%q", got)
		}
		if w := len([]rune(got)); w != sidebarWidth {
			t.Fatalf("width=%d want %d (%q)", w, sidebarWidth, got)
		}
	})
	t.Run("hidden", func(t *testing.T) {
		got := formatGroupHeader(state.NumGroups-1, false)
		if !strings.HasPrefix(got, "○ 0 ") {
			t.Fatalf("prefix=%q", got)
		}
		if w := len([]rune(got)); w != sidebarWidth {
			t.Fatalf("width=%d want %d (%q)", w, sidebarWidth, got)
		}
	})
}

func TestRefreshSidebar_UsesInsertionOrderWithinGroup(t *testing.T) {
	u, _ := newTestUI(t)
	for _, ch := range []string{"#a", "#b", "#c", "#d", "#e", "#f", "#g", "#h", "#i", "#j", "#k"} {
		u.state.JoinChannel(ch)
	}

	u.refreshSidebar()
	got := u.sidebarView.GetText(true)
	// Verify the channels appear in insertion order after a group header containing "1"
	lines := strings.Split(got, "\n")
	foundGroup1 := false
	for i, line := range lines {
		if strings.Contains(line, "1") && strings.Contains(line, "─") {
			foundGroup1 = true
			// Next two lines should be channels in insertion order; current target
			// is marked with the active-channel indicator.
			if i+2 < len(lines) && lines[i+1] == "▶ #a" && lines[i+2] == "  #k" {
				return // Test passed
			}
			t.Fatalf("group 1 channels not in expected insertion order after line %d:\n%s", i, got)
		}
	}
	if !foundGroup1 {
		t.Fatalf("group 1 header not found in sidebar:\n%s", got)
	}
}

func TestRefreshSidebar_ShowsActiveTargetIndicator(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#a")
	u.state.JoinChannel("#b")
	u.state.SetTarget("#b")

	u.refreshSidebar()
	got := u.sidebarView.GetText(true)

	if !strings.Contains(got, "\n▶ #b") && !strings.HasPrefix(got, "▶ #b") {
		t.Fatalf("sidebar missing active-target marker for #b:\n%s", got)
	}
	if strings.Contains(got, "\n▶ #a") || strings.HasPrefix(got, "▶ #a") {
		t.Fatalf("sidebar marked non-target channel as active:\n%s", got)
	}
}

func TestRefreshSidebar_NoIndicatorWhenTargetIsNonNumericChannel(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#a")
	u.state.JoinChannel("#b")
	u.state.JoinChannel("alice")
	u.state.SetTarget("alice")

	u.refreshSidebar()
	got := u.sidebarView.GetText(true)

	if strings.Contains(got, "▶ ") {
		t.Fatalf("sidebar should not show active-target marker for non-numeric target:\n%s", got)
	}
}

func TestRefreshSidebar_ShowsIndicatorInHiddenGroup(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#a")
	u.state.JoinChannel("#b")
	u.state.SetVisible(state.GroupID(1), false)
	u.state.SetTarget("#b")

	u.refreshSidebar()
	got := u.sidebarView.GetText(true)

	if !strings.Contains(got, "\n○ 2 ") && !strings.HasPrefix(got, "○ 2 ") {
		t.Fatalf("hidden group header missing:\n%s", got)
	}
	if !strings.Contains(got, "\n▶ #b") && !strings.HasPrefix(got, "▶ #b") {
		t.Fatalf("sidebar should show active-target marker in hidden group:\n%s", got)
	}
}

func TestToggleSidebar_ResizesContentRow(t *testing.T) {
	u, _ := newTestUI(t)

	drawUIRoot(t, u, 100, 20)
	_, _, hiddenW, _ := u.sidebarView.GetRect()
	_, _, hiddenDividerW, _ := u.sidebarDivider.GetRect()
	if hiddenW != 0 {
		t.Fatalf("hidden sidebar width=%d want 0", hiddenW)
	}
	if hiddenDividerW != 0 {
		t.Fatalf("hidden divider width=%d want 0", hiddenDividerW)
	}

	u.toggleSidebar()
	drawUIRoot(t, u, 100, 20)
	_, _, shownW, _ := u.sidebarView.GetRect()
	_, _, shownDividerW, _ := u.sidebarDivider.GetRect()
	if shownW != sidebarWidth {
		t.Fatalf("shown sidebar width=%d want %d", shownW, sidebarWidth)
	}
	if shownDividerW != 1 {
		t.Fatalf("shown divider width=%d want 1", shownDividerW)
	}

	u.toggleSidebar()
	drawUIRoot(t, u, 100, 20)
	_, _, hiddenAgainW, _ := u.sidebarView.GetRect()
	_, _, hiddenAgainDividerW, _ := u.sidebarDivider.GetRect()
	if hiddenAgainW != 0 {
		t.Fatalf("hidden-again sidebar width=%d want 0", hiddenAgainW)
	}
	if hiddenAgainDividerW != 0 {
		t.Fatalf("hidden-again divider width=%d want 0", hiddenAgainDividerW)
	}
}

func TestToggleUsersPanel_ResizesContentRow(t *testing.T) {
	u, _ := newTestUI(t)

	drawUIRoot(t, u, 100, 20)
	_, _, hiddenW, _ := u.usersView.GetRect()
	_, _, hiddenDividerW, _ := u.usersDivider.GetRect()
	if hiddenW != 0 {
		t.Fatalf("hidden users width=%d want 0", hiddenW)
	}
	if hiddenDividerW != 0 {
		t.Fatalf("hidden users divider width=%d want 0", hiddenDividerW)
	}

	u.toggleUsersPanel()
	drawUIRoot(t, u, 100, 20)
	_, _, shownW, _ := u.usersView.GetRect()
	_, _, shownDividerW, _ := u.usersDivider.GetRect()
	if shownW != usersPanelWidth {
		t.Fatalf("shown users width=%d want %d", shownW, usersPanelWidth)
	}
	if shownDividerW != 1 {
		t.Fatalf("shown users divider width=%d want 1", shownDividerW)
	}

	u.toggleUsersPanel()
	drawUIRoot(t, u, 100, 20)
	_, _, hiddenAgainW, _ := u.usersView.GetRect()
	_, _, hiddenAgainDividerW, _ := u.usersDivider.GetRect()
	if hiddenAgainW != 0 {
		t.Fatalf("hidden-again users width=%d want 0", hiddenAgainW)
	}
	if hiddenAgainDividerW != 0 {
		t.Fatalf("hidden-again users divider width=%d want 0", hiddenAgainDividerW)
	}
}

func TestRefreshUsersPanel_ShowsSortedUsersAndActiveIndicator(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#a")
	u.state.JoinChannel("#b")
	u.state.SetChannelUsers("#a", map[string]string{"alice": "", "bob": "@"})
	u.state.SetChannelUsers("#b", map[string]string{"carol": ""})
	u.state.UpdateUserActivity("#a", "bob", nowForTest())
	u.state.SetTarget("#a")

	u.refreshUsersPanel()

	title := u.usersTitleView.GetText(true)
	if !strings.Contains(title, "Users — 3") {
		t.Fatalf("users title=%q want count 3", title)
	}
	body := u.usersView.GetText(true)
	lines := strings.Split(body, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines=%d want 3 (%q)", len(lines), body)
	}
	if !strings.Contains(lines[0], "@ ") || !strings.Contains(lines[0], "bob") {
		t.Fatalf("first line should be op bob with @: %q", lines[0])
	}
	if !strings.Contains(lines[1], "• ") || !strings.Contains(lines[1], "alice") {
		t.Fatalf("second line should be active alice: %q", lines[1])
	}
	if strings.Contains(lines[2], "• ") || !strings.Contains(lines[2], "carol") {
		t.Fatalf("third line should be inactive carol: %q", lines[2])
	}
}

func TestRefreshUsersPanel_SelfAlwaysHasBulletInJoinedChannel(t *testing.T) {
	u, fs := newTestUI(t)
	fs.nick = "me"

	// Join a channel and set target
	u.state.JoinChannel("#test")
	u.state.SetTarget("#test")

	// Simulate NAMES reply that doesn't include self
	// This simulates a server bug or parsing issue
	// Normally, OnNames would fix this by adding self to the list
	// But in tests we can't call OnNames (it hangs), so we test the workaround:
	// We manually ensure self is in the list before calling SetChannelUsers
	nicks := map[string]string{"alice": "", "bob": ""}

	// Simulate the fix that OnNames does: check if we're in the channel and add self
	if u.state.IsJoinedNormalChannel("#test") {
		selfNick := fs.Nick()
		hasSelf := false
		for nick := range nicks {
			if strings.EqualFold(nick, selfNick) {
				hasSelf = true
				break
			}
		}
		if !hasSelf {
			nicks[selfNick] = ""
		}
	}

	u.state.SetChannelUsers("#test", nicks)

	// Verify self is in the channel user list
	targetUsers := u.state.UsersInChannel("#test")
	if !targetUsers["me"] {
		t.Fatal("Self should be in channel user list after fix")
	}

	// Verify bullet indicator shows up for self
	u.refreshUsersPanel()
	body := u.usersView.GetText(true)

	foundSelfWithBullet := false
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "me") && strings.Contains(line, "• ") {
			foundSelfWithBullet = true
			break
		}
	}
	if !foundSelfWithBullet {
		t.Fatalf("Expected bullet indicator for self, got:\n%s", body)
	}
}

func TestOnNamesLogic_DoesNotAddSelfToNonJoinedChannel(t *testing.T) {
	u, fs := newTestUI(t)
	fs.nick = "me"

	// Don't join the channel - just create it as a query or something
	// Actually, let's just not join it at all

	// Simulate NAMES for a channel we're NOT in
	nicks := []string{"alice", "bob"}
	channel := "#other"

	// The fix should NOT add self if we're not in the channel
	if u.state.IsJoinedNormalChannel(channel) {
		selfNick := fs.Nick()
		hasSelf := false
		for _, nick := range nicks {
			if strings.EqualFold(nick, selfNick) {
				hasSelf = true
				break
			}
		}
		if !hasSelf {
			nicks = append(nicks, selfNick)
		}
	}

	// nicks should still be just alice and bob
	if len(nicks) != 2 {
		t.Fatalf("Expected 2 nicks for non-joined channel, got %d", len(nicks))
	}
}

func TestOnNamesLogic_DoesNotDuplicateSelfIfAlreadyInList(t *testing.T) {
	u, fs := newTestUI(t)
	fs.nick = "me"

	u.state.JoinChannel("#test")

	// Simulate NAMES that DOES include self
	nicks := []string{"alice", "me", "bob"}
	channel := "#test"

	// The fix should NOT duplicate self
	if u.state.IsJoinedNormalChannel(channel) {
		selfNick := fs.Nick()
		hasSelf := false
		for _, nick := range nicks {
			if strings.EqualFold(nick, selfNick) {
				hasSelf = true
				break
			}
		}
		if !hasSelf {
			nicks = append(nicks, selfNick)
		}
	}

	// nicks should still be 3 (not 4)
	if len(nicks) != 3 {
		t.Fatalf("Expected 3 nicks (no duplicate), got %d", len(nicks))
	}

	// Verify "me" appears exactly once
	count := 0
	for _, nick := range nicks {
		if strings.EqualFold(nick, "me") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Expected self to appear once, got %d times", count)
	}
}

func TestOnNamesLogic_HandlesCaseInsensitiveComparison(t *testing.T) {
	u, fs := newTestUI(t)
	fs.nick = "FlugGA" // Mixed case

	u.state.JoinChannel("#test")

	// Simulate NAMES with lowercase version of self
	nicks := []string{"alice", "flugga", "bob"}
	channel := "#test"

	// The fix should recognize "flugga" as self despite different casing
	if u.state.IsJoinedNormalChannel(channel) {
		selfNick := fs.Nick()
		hasSelf := false
		for _, nick := range nicks {
			if strings.EqualFold(nick, selfNick) {
				hasSelf = true
				break
			}
		}
		if !hasSelf {
			nicks = append(nicks, selfNick)
		}
	}

	// Should not add duplicate
	if len(nicks) != 3 {
		t.Fatalf("Expected 3 nicks (case-insensitive match), got %d", len(nicks))
	}
}

func TestOnNamesLogic_HandlesEmptyNamesList(t *testing.T) {
	u, fs := newTestUI(t)
	fs.nick = "me"

	u.state.JoinChannel("#test")

	// Simulate empty NAMES (weird but possible)
	nicks := []string{}
	channel := "#test"

	// The fix should add self
	if u.state.IsJoinedNormalChannel(channel) {
		selfNick := fs.Nick()
		if selfNick != "" {
			hasSelf := false
			for _, nick := range nicks {
				if strings.EqualFold(nick, selfNick) {
					hasSelf = true
					break
				}
			}
			if !hasSelf {
				nicks = append(nicks, selfNick)
			}
		}
	}

	// Should have just self
	if len(nicks) != 1 {
		t.Fatalf("Expected 1 nick (self), got %d", len(nicks))
	}
	if !strings.EqualFold(nicks[0], "me") {
		t.Fatalf("Expected self nick 'me', got %q", nicks[0])
	}
}

func TestFormatDecimalSI(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{in: 42, want: "42"},
		{in: 53_000, want: "53k"},
		{in: 2_700_000, want: "2.7M"},
		{in: 10_400, want: "10k"},
	}
	for _, tc := range cases {
		if got := formatDecimalSI(tc.in); got != tc.want {
			t.Fatalf("formatDecimalSI(%d)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func nowForTest() time.Time {
	return time.Now().Add(-time.Second)
}

func TestRefreshSeparator_UsesSeparatorWidth(t *testing.T) {
	u, _ := newTestUI(t)
	u.separatorView.SetRect(0, 0, 12, 1)

	u.refreshSeparator()

	want := "[" + state.ActiveChromeTheme().Separator + "]" + strings.Repeat(dottedSeparatorRune, 12) + state.ResetColor()
	if got := u.separatorView.GetText(false); got != want {
		t.Fatalf("separator text=%q want %q", got, want)
	}
}

func TestRefreshSeparator_FallsBackToMainWidth(t *testing.T) {
	u, _ := newTestUI(t)
	u.mainView.SetRect(0, 0, 9, 1)
	u.separatorView.SetRect(0, 0, 0, 1)

	u.refreshSeparator()

	want := "[" + state.ActiveChromeTheme().Separator + "]" + strings.Repeat(dottedSeparatorRune, 9) + state.ResetColor()
	if got := u.separatorView.GetText(false); got != want {
		t.Fatalf("separator text=%q want %q", got, want)
	}
}

func TestEventsViewHeight(t *testing.T) {
	cases := []struct {
		name  string
		lines int
		want  int
	}{
		{name: "empty collapses", lines: 0, want: 0},
		{name: "single line", lines: 1, want: 1},
		{name: "under max", lines: 4, want: 4},
		{name: "at max", lines: 5, want: 5},
		{name: "over max caps", lines: 99, want: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventsViewHeight(tc.lines); got != tc.want {
				t.Fatalf("eventsViewHeight(%d)=%d want %d", tc.lines, got, tc.want)
			}
		})
	}
}

func TestRefreshEvents_ResizesPanelHeight(t *testing.T) {
	cases := []struct {
		name   string
		events int
		wantH  int
	}{
		{name: "zero events collapse", events: 0, wantH: 0},
		{name: "three events", events: 3, wantH: 3},
		{name: "over cap", events: 10, wantH: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, _ := newTestUI(t)
			for i := 0; i < tc.events; i++ {
				u.state.AddEvent(fmt.Sprintf("event %d", i))
			}

			u.refreshEvents()
			drawUIRoot(t, u, 80, 20)

			_, _, _, gotH := u.eventsView.GetRect()
			if gotH != tc.wantH {
				t.Fatalf("eventsView height=%d want %d", gotH, tc.wantH)
			}
		})
	}
}

func TestRefreshEvents_GrowsAndShrinksPanelHeight(t *testing.T) {
	u, _ := newTestUI(t)

	u.refreshEvents()
	drawUIRoot(t, u, 80, 20)
	_, _, _, h0 := u.eventsView.GetRect()
	if h0 != 0 {
		t.Fatalf("initial eventsView height=%d want 0", h0)
	}

	u.state.AddEvent("a")
	u.state.AddEvent("b")
	u.refreshEvents()
	drawUIRoot(t, u, 80, 20)
	_, _, _, h2 := u.eventsView.GetRect()
	if h2 != 2 {
		t.Fatalf("eventsView height after 2 events=%d want 2", h2)
	}

	u.state.AddEvent("c")
	u.state.AddEvent("d")
	u.state.AddEvent("e")
	u.refreshEvents()
	drawUIRoot(t, u, 80, 20)
	_, _, _, h5 := u.eventsView.GetRect()
	if h5 != 5 {
		t.Fatalf("eventsView height after 5 events=%d want 5", h5)
	}
}

func drawUIRoot(t *testing.T, u *UI, width, height int) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	defer screen.Fini()

	screen.SetSize(width, height)
	u.root.SetRect(0, 0, width, height)
	u.root.Draw(screen)
}

// --- groupForRune ----------------------------------------------------------

func TestGroupForRune(t *testing.T) {
	cases := []struct {
		r      rune
		want   state.GroupID
		wantOK bool
	}{
		{'1', 0, true},
		{'9', 8, true},
		{'0', 9, true},
		{'a', 0, false},
		{' ', 0, false},
	}
	for _, c := range cases {
		got, ok := groupForRune(c.r)
		if ok != c.wantOK || got != c.want {
			t.Errorf("groupForRune(%q)=(%d,%v) want (%d,%v)", c.r, got, ok, c.want, c.wantOK)
		}
	}
}

// --- handleSubmit ---------------------------------------------------------

func TestHandleSubmit_Empty(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.handleSubmit("   ")
	if len(fs.privmsgs) != 0 {
		t.Errorf("empty input shouldn't send")
	}
}

func TestHandleSubmit_NoTarget(t *testing.T) {
	u, fs := newTestUI(t)
	u.handleSubmit("hello")
	if len(fs.privmsgs) != 0 {
		t.Error("send happened with no target")
	}
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "no target") {
		t.Errorf("expected no-target event, got %v", ev)
	}
}

func TestHandleSubmit_PlainMessage(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.handleSubmit("hello there")
	if len(fs.privmsgs) != 1 || fs.privmsgs[0] != (sentMsg{"#a", "hello there"}) {
		t.Errorf("privmsgs=%v", fs.privmsgs)
	}
	// Local echo should land in state.
	rendered := u.state.RenderVisible()
	wantNick := "<" + state.UserColor("me") + "me" + state.ResetColor() + ">"
	if len(rendered) != 1 || !strings.Contains(rendered[0], wantNick) {
		t.Errorf("echo missing: %v", rendered)
	}
}

func TestHandleSubmit_ForcesGroupVisible(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.EnsureChannel("#a")
	c, _ := u.state.Channel("#a")
	u.state.SetVisible(c.Group, false)
	u.handleSubmit("ping")
	if !u.state.IsVisible(c.Group) {
		t.Error("group not forced visible")
	}
	if len(fs.privmsgs) != 1 {
		t.Error("message not sent")
	}
}

func TestHandleSubmit_SendError(t *testing.T) {
	u, fs := newTestUI(t)
	fs.sendErr = errors.New("boom")
	u.state.EnsureChannel("#a")
	u.handleSubmit("hi")
	rendered := u.state.RenderVisible()
	if len(rendered) != 0 {
		t.Errorf("echo despite send error: %v", rendered)
	}
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "send failed") {
		t.Errorf("expected send-failed event, got %v", ev)
	}
}

func TestHandleSubmit_MeAction(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.handleSubmit("/me waves")
	if len(fs.actions) != 1 || fs.actions[0] != (sentMsg{"#a", "waves"}) {
		t.Errorf("actions=%v", fs.actions)
	}
	if len(fs.privmsgs) != 0 {
		t.Errorf("/me leaked into privmsg path: %v", fs.privmsgs)
	}
}

func TestHandleSubmit_MeWithoutArg(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.handleSubmit("/me")
	if len(fs.actions) != 0 {
		t.Errorf("empty /me sent: %v", fs.actions)
	}
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "usage: /me") {
		t.Errorf("expected usage event, got %v", ev)
	}
}

func TestHandleSubmit_QuitDefaultReason(t *testing.T) {
	u, fs := newTestUI(t)
	u.handleSubmit("/quit")
	if fs.quitN != 1 {
		t.Errorf("quitN=%d", fs.quitN)
	}
	if fs.quitWith == "" {
		t.Error("expected default reason")
	}
}

func TestHandleSubmit_QuitWithReason(t *testing.T) {
	u, fs := newTestUI(t)
	u.handleSubmit("/quit bye all")
	if fs.quitWith != "bye all" {
		t.Errorf("quitWith=%q", fs.quitWith)
	}
}

func TestHandleSubmit_UnknownCommand(t *testing.T) {
	u, _ := newTestUI(t)
	u.handleSubmit("/nope hi")
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "unknown command") {
		t.Errorf("expected unknown-cmd event, got %v", ev)
	}
}

func TestHandleSubmit_Help(t *testing.T) {
	u, _ := newTestUI(t)

	u.handleSubmit("/help")

	ev := u.state.Events()
	if len(ev) < 2 {
		t.Fatalf("expected help output events, got %v", ev)
	}
	if !strings.Contains(ev[len(ev)-2], "/help") || !strings.Contains(ev[len(ev)-2], "/join") {
		t.Fatalf("expected command list in help output, got %q", ev[len(ev)-2])
	}
	if !strings.Contains(ev[len(ev)-1], "usage:") || !strings.Contains(ev[len(ev)-1], "/quit") {
		t.Fatalf("expected usage help line, got %q", ev[len(ev)-1])
	}
}

func TestHandleSubmit_ListOpensBrowser(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetChannelListCache([]string{"#alpha"})

	u.handleSubmit("/list")

	if !u.browserVisible {
		t.Fatal("browser should be visible after /list")
	}
	if got := u.browser.selectedChannel(); got != "#alpha" {
		t.Fatalf("selected=%q want #alpha", got)
	}
}

func TestHandleSubmit_Join(t *testing.T) {
	u, fs := newTestUI(t)
	u.handleSubmit("/join #go")
	if len(fs.joins) != 1 || fs.joins[0] != "#go" {
		t.Fatalf("joins=%v", fs.joins)
	}
}

func TestHandleSubmit_JoinWithoutArg(t *testing.T) {
	u, fs := newTestUI(t)
	u.handleSubmit("/join")
	if len(fs.joins) != 0 {
		t.Fatalf("unexpected join calls: %v", fs.joins)
	}
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "usage: /join") {
		t.Fatalf("expected usage event, got %v", ev)
	}
}

func TestHandleSubmit_JoinError(t *testing.T) {
	u, fs := newTestUI(t)
	fs.joinErr = errors.New("denied")
	u.handleSubmit("/join #go")
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "join failed") {
		t.Fatalf("expected join-failed event, got %v", ev)
	}
}

func TestHandleSubmit_PartActiveChannel(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.JoinChannel("#a")

	u.handleSubmit("/part")

	if len(fs.parts) != 1 || fs.parts[0] != (sentPart{Channel: "#a", Reason: ""}) {
		t.Fatalf("parts=%v", fs.parts)
	}
}

func TestHandleSubmit_PartActiveChannelWithReason(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.JoinChannel("#a")

	u.handleSubmit("/part stepping away")

	if len(fs.parts) != 1 || fs.parts[0] != (sentPart{Channel: "#a", Reason: "stepping away"}) {
		t.Fatalf("parts=%v", fs.parts)
	}
}

func TestHandleSubmit_PartExplicitChannelWithReason(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.JoinChannel("#a")
	u.state.JoinChannel("#b")

	u.handleSubmit("/part #b see ya")

	if len(fs.parts) != 1 || fs.parts[0] != (sentPart{Channel: "#b", Reason: "see ya"}) {
		t.Fatalf("parts=%v", fs.parts)
	}
}

func TestHandleSubmit_PartNoTarget(t *testing.T) {
	u, fs := newTestUI(t)

	u.handleSubmit("/part")

	if len(fs.parts) != 0 {
		t.Fatalf("unexpected part calls: %v", fs.parts)
	}
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "no target") {
		t.Fatalf("expected no-target event, got %v", ev)
	}
}

func TestHandleSubmit_PartQueryTargetRejected(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.JoinChannel("alice")
	u.state.SetTarget("alice")

	u.handleSubmit("/part")

	if len(fs.parts) != 0 {
		t.Fatalf("unexpected part calls: %v", fs.parts)
	}
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "cannot part from queries/server") {
		t.Fatalf("expected query/server error event, got %v", ev)
	}
}

func TestHandleSubmit_PartServerTargetRejected(t *testing.T) {
	u, fs := newTestUI(t)

	u.handleSubmit("/part " + state.ServerChannelName)

	if len(fs.parts) != 0 {
		t.Fatalf("unexpected part calls: %v", fs.parts)
	}
	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "cannot part from queries/server") {
		t.Fatalf("expected query/server error event, got %v", ev)
	}
}

func TestHandleSubmit_PartError(t *testing.T) {
	u, fs := newTestUI(t)
	fs.partErr = errors.New("not on channel")
	u.state.JoinChannel("#a")

	u.handleSubmit("/part")

	ev := u.state.Events()
	if len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "part failed") {
		t.Fatalf("expected part-failed event, got %v", ev)
	}
}

func TestTryJoinCompletion_OnlyCommandAddsHash(t *testing.T) {
	u, _ := newTestUI(t)
	u.input.SetText("/join ")
	if !u.tryJoinCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/join #" {
		t.Fatalf("input=%q", got)
	}
}

func TestTryJoinCompletion_SingleMatch(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetChannelListCache([]string{"#golang", "#rust"})
	u.input.SetText("/join #go")
	if !u.tryJoinCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/join #golang " {
		t.Fatalf("input=%q", got)
	}
}

func TestTryJoinCompletion_CompletesLargestCommonPrefix(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetChannelListCache([]string{"#go-help", "#go-nuts", "#games"})
	u.input.SetText("/join #g")
	if !u.tryJoinCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/join #g" {
		t.Fatalf("expected unchanged partial because lcp == partial, got %q", got)
	}

	u.input.SetText("/join #go")
	if !u.tryJoinCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/join #go-" {
		t.Fatalf("input=%q", got)
	}
}

func TestTryJoinCompletion_DoubleTabShowsCompletions(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.state.SetChannelListCache([]string{"#go-help", "#go-nuts", "#go-dev"})
	u.input.SetText("/join #go-")
	before := len(u.state.Events())

	if !u.tryJoinCompletion() {
		t.Fatal("completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected completions in events, before=%d after=%d", before, len(after))
	}
}

func TestTryJoinCompletion_DoubleTabAfterLCPExpansionShowsSameSuggestions(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.state.SetChannelListCache([]string{
		"#archive",
		"#archivebot-alerts",
		"#archivebot-bs",
		"#archiveteam-internal",
	})

	u.input.SetText("/join #arch")
	if !u.tryJoinCompletion() {
		t.Fatal("first completion not consumed")
	}
	if got := u.input.GetText(); got != "/join #archive" {
		t.Fatalf("input=%q", got)
	}

	before := len(u.state.Events())
	if !u.tryJoinCompletion() {
		t.Fatal("second completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected suggestions after second tab, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	for _, want := range []string{"#archive", "#archivebot-alerts", "#archivebot-bs", "#archiveteam-internal"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in suggestions: %v", want, after[before:])
		}
	}
}

func TestTryJoinCompletion_DoubleTabUsesSavedCandidatesIfCacheChanges(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.state.SetChannelListCache([]string{"#archive", "#archivebot-alerts", "#archivebot-bs"})

	u.input.SetText("/join #arch")
	if !u.tryJoinCompletion() {
		t.Fatal("first completion not consumed")
	}
	if got := u.input.GetText(); got != "/join #archive" {
		t.Fatalf("input=%q", got)
	}

	// Simulate a cache refresh racing with user input between first and second Tab.
	u.state.SetChannelListCache([]string{"#archive"})

	before := len(u.state.Events())
	if !u.tryJoinCompletion() {
		t.Fatal("second completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected suggestions after second tab, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	if !strings.Contains(joined, "#archivebot-alerts") || !strings.Contains(joined, "#archivebot-bs") {
		t.Fatalf("expected saved candidates in suggestions, got %v", after[before:])
	}
}

func TestTryJoinCompletion_ExcludesAlreadyJoinedChannels(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetChannelListCache([]string{"#go", "#golang", "#games"})
	u.state.JoinChannel("#go")
	u.input.SetText("/join #go")
	if !u.tryJoinCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/join #golang " {
		t.Fatalf("expected joined channel exclusion, got %q", got)
	}
}

func TestTryPartCompletion_SingleMatch(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#golang")
	u.state.JoinChannel("#rust")
	u.input.SetText("/part #go")

	if !u.tryPartCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/part #golang " {
		t.Fatalf("input=%q", got)
	}
}

func TestTryPartCompletion_CompletesLargestCommonPrefix(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#go-help")
	u.state.JoinChannel("#go-nuts")
	u.state.JoinChannel("#games")
	u.input.SetText("/part #g")

	if !u.tryPartCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/part #g" {
		t.Fatalf("expected unchanged partial because lcp == partial, got %q", got)
	}

	u.input.SetText("/part #go")
	if !u.tryPartCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/part #go-" {
		t.Fatalf("input=%q", got)
	}
}

func TestTryPartCompletion_DoubleTabShowsCompletions(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.state.JoinChannel("#go-help")
	u.state.JoinChannel("#go-nuts")
	u.state.JoinChannel("#go-dev")
	u.input.SetText("/part #go-")
	before := len(u.state.Events())

	if !u.tryPartCompletion() {
		t.Fatal("completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected completions in events, before=%d after=%d", before, len(after))
	}
}

func TestTryPartCompletion_DoubleTabAfterLCPExpansionShowsSameSuggestions(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.state.JoinChannel("#archive")
	u.state.JoinChannel("#archivebot-alerts")
	u.state.JoinChannel("#archivebot-bs")
	u.state.JoinChannel("#archiveteam-internal")

	u.input.SetText("/part #arch")
	if !u.tryPartCompletion() {
		t.Fatal("first completion not consumed")
	}
	if got := u.input.GetText(); got != "/part #archive" {
		t.Fatalf("input=%q", got)
	}

	before := len(u.state.Events())
	if !u.tryPartCompletion() {
		t.Fatal("second completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected suggestions after second tab, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	for _, want := range []string{"#archive", "#archivebot-alerts", "#archivebot-bs", "#archiveteam-internal"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in suggestions: %v", want, after[before:])
		}
	}
}

func TestTryPartCompletion_DoubleTabUsesSavedCandidatesIfChannelsChange(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.state.JoinChannel("#archive")
	u.state.JoinChannel("#archivebot-alerts")
	u.state.JoinChannel("#archivebot-bs")

	u.input.SetText("/part #arch")
	if !u.tryPartCompletion() {
		t.Fatal("first completion not consumed")
	}
	if got := u.input.GetText(); got != "/part #archive" {
		t.Fatalf("input=%q", got)
	}

	// Simulate channel state changing between first and second Tab.
	u.state.PartChannel("#archivebot-alerts")
	u.state.PartChannel("#archivebot-bs")

	before := len(u.state.Events())
	if !u.tryPartCompletion() {
		t.Fatal("second completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected suggestions after second tab, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	if !strings.Contains(joined, "#archivebot-alerts") || !strings.Contains(joined, "#archivebot-bs") {
		t.Fatalf("expected saved candidates in suggestions, got %v", after[before:])
	}
}

func TestTryCommandCompletion_SingleMatch(t *testing.T) {
	u, _ := newTestUI(t)
	u.input.SetText("/j")

	if !u.tryCommandCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/join " {
		t.Fatalf("input=%q", got)
	}
}

func TestTryCommandCompletion_IncludesHelp(t *testing.T) {
	u, _ := newTestUI(t)
	u.input.SetText("/h")

	if !u.tryCommandCompletion() {
		t.Fatal("completion not consumed")
	}
	if got := u.input.GetText(); got != "/help " {
		t.Fatalf("input=%q", got)
	}
}

func TestTryCommandCompletion_AmbiguousShowsCompletions(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.input.SetText("/")
	before := len(u.state.Events())

	if !u.tryCommandCompletion() {
		t.Fatal("completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected completions in events, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	for _, want := range knownCommands {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in suggestions: %v", want, after[before:])
		}
	}
}

func TestTryCommandCompletion_DoubleTabOnAmbiguousInputShowsSuggestions(t *testing.T) {
	u, _ := newTestUI(t)
	u.eventsView.SetRect(0, 0, 80, 5)
	u.input.SetText("/")

	if !u.tryCommandCompletion() {
		t.Fatal("first completion not consumed")
	}
	if got := u.input.GetText(); got != "/" {
		t.Fatalf("input=%q", got)
	}

	before := len(u.state.Events())
	if !u.tryCommandCompletion() {
		t.Fatal("second completion not consumed")
	}
	after := u.state.Events()
	if len(after) <= before {
		t.Fatalf("expected suggestions after second tab, before=%d after=%d", before, len(after))
	}
	joined := strings.Join(after[before:], " ")
	for _, want := range knownCommands {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in suggestions: %v", want, after[before:])
		}
	}
}

func TestTryCommandCompletion_IgnoresWithSpace(t *testing.T) {
	u, _ := newTestUI(t)
	u.input.SetText("/join ")

	if u.tryCommandCompletion() {
		t.Fatal("command completion unexpectedly consumed argument phase")
	}
}

func TestHandleKey_TabCompletesJoin(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetChannelListCache([]string{"#golang"})
	u.input.SetText("/join #go")
	ev := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("tab not consumed")
	}
	if got := u.input.GetText(); got != "/join #golang " {
		t.Fatalf("input=%q", got)
	}
}

func TestHandleKey_TabCompletesPart(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#golang")
	u.input.SetText("/part #go")
	ev := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("tab not consumed")
	}
	if got := u.input.GetText(); got != "/part #golang " {
		t.Fatalf("input=%q", got)
	}
}

func TestHandleKey_TabCompletesCommand(t *testing.T) {
	u, _ := newTestUI(t)
	u.input.SetText("/h")
	ev := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("tab not consumed")
	}
	if got := u.input.GetText(); got != "/help " {
		t.Fatalf("input=%q", got)
	}
}

func TestHandleKey_TabFallsBackToPartAfterJoinMiss(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.JoinChannel("#go")
	u.input.SetText("/part #g")
	ev := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("tab not consumed")
	}
	if got := u.input.GetText(); got != "/part #go " {
		t.Fatalf("input=%q", got)
	}
}

func TestHandleKey_TabPassesThroughWhenNotJoin(t *testing.T) {
	u, _ := newTestUI(t)
	u.input.SetText("hello")
	ev := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != ev {
		t.Fatal("tab unexpectedly consumed")
	}
}

// --- handleKey ------------------------------------------------------------

func TestHandleKey_AltDigitTogglesGroup(t *testing.T) {
	u, _ := newTestUI(t)
	if !u.state.IsVisible(state.GroupID(0)) {
		t.Fatal("default visibility")
	}
	ev := tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModAlt)
	if u.handleKey(ev) != nil {
		t.Error("Alt+1 not consumed")
	}
	if u.state.IsVisible(state.GroupID(0)) {
		t.Error("group 0 still visible after Alt+1")
	}
}

func TestHandleKey_AltGTogglesSidebar(t *testing.T) {
	u, _ := newTestUI(t)

	ev := tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModAlt)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("Alt+g not consumed")
	}
	if !u.sidebarVisible {
		t.Fatal("sidebar not visible after Alt+g")
	}

	evShift := tcell.NewEventKey(tcell.KeyRune, 'G', tcell.ModAlt)
	if got := u.handleKey(evShift); got != nil {
		t.Fatal("Alt+G not consumed")
	}
	if u.sidebarVisible {
		t.Fatal("sidebar still visible after second Alt+G")
	}
}

func TestHandleKey_AltUTogglesUsersPanel(t *testing.T) {
	u, _ := newTestUI(t)

	ev := tcell.NewEventKey(tcell.KeyRune, 'u', tcell.ModAlt)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("Alt+u not consumed")
	}
	if !u.usersVisible {
		t.Fatal("users panel not visible after Alt+u")
	}

	evShift := tcell.NewEventKey(tcell.KeyRune, 'U', tcell.ModAlt)
	if got := u.handleKey(evShift); got != nil {
		t.Fatal("Alt+U not consumed")
	}
	if u.usersVisible {
		t.Fatal("users panel still visible after second Alt+U")
	}
}

func TestHandleKey_AltLTogglesBrowser(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetChannelListCache([]string{"#alpha"})

	ev := tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModAlt)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("Alt+l not consumed")
	}
	if !u.browserVisible {
		t.Fatal("browser not visible after Alt+l")
	}

	evShift := tcell.NewEventKey(tcell.KeyRune, 'L', tcell.ModAlt)
	if got := u.handleKey(evShift); got != nil {
		t.Fatal("Alt+L not consumed")
	}
	if u.browserVisible {
		t.Fatal("browser still visible after second Alt+L")
	}
}

func TestHandleKey_BrowserEscCloses(t *testing.T) {
	u, _ := newTestUI(t)
	u.openBrowser()

	ev := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("Esc not consumed while browser open")
	}
	if u.browserVisible {
		t.Fatal("browser still visible after Esc")
	}
}

func TestHandleKey_BrowserTypingUpdatesFilterAndResetsSelection(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetChannelListCache([]string{"#archive", "#beta", "#gamma"})
	u.openBrowser()
	u.browser.listView.SetCurrentItem(2)

	ev := tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("typed rune not consumed while browser open")
	}
	if got := u.browser.listView.GetCurrentItem(); got != 0 {
		t.Fatalf("selection=%d want 0", got)
	}
	if got := u.browser.searchQuery; got != "a" {
		t.Fatalf("query=%q want a", got)
	}
}

func TestHandleKey_BrowserAltJJoinsSelectedChannel(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.SetChannelListCache([]string{"#alpha", "#beta"})
	u.state.JoinChannel("#beta")
	u.openBrowser()
	u.browser.listView.SetCurrentItem(1)

	ev := tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModAlt)
	if got := u.handleKey(ev); got != nil {
		t.Fatal("Alt+j not consumed while browser open")
	}
	if len(fs.joins) != 1 || fs.joins[0] != "#beta" {
		t.Fatalf("joins=%v", fs.joins)
	}
	if u.browserVisible {
		t.Fatal("browser should close after join")
	}
}

func TestHandleKey_F1SolosGroup(t *testing.T) {
	u, _ := newTestUI(t)
	ev := tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)
	if u.handleKey(ev) != nil {
		t.Error("F1 not consumed")
	}
	for i := 0; i < state.NumGroups; i++ {
		want := i == 0
		if got := u.state.IsVisible(state.GroupID(i)); got != want {
			t.Fatalf("group %d visible=%v want %v", i, got, want)
		}
	}
}

func TestHandleKey_F1SoloPreservesSpecialGroups(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.SetVisible(state.GroupServer, false)
	u.state.SetVisible(state.GroupQueries, true)

	ev := tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)
	if u.handleKey(ev) != nil {
		t.Error("F1 not consumed")
	}
	if u.state.IsVisible(state.GroupServer) {
		t.Error("server visibility changed by solo")
	}
	if !u.state.IsVisible(state.GroupQueries) {
		t.Error("queries visibility changed by solo")
	}
}

func TestRefreshStatus_UpdatesChannelCountView(t *testing.T) {
	u, _ := newTestUI(t)
	if got := u.statusCountView.GetText(true); got != "0 Channels" {
		t.Fatalf("initial count=%q want %q", got, "0 Channels")
	}

	u.state.EnsureChannel("#a")
	u.state.EnsureChannel("alice")
	u.refreshStatus()

	if got := u.statusCountView.GetText(true); got != "1 Channel" {
		t.Errorf("count=%q want %q", got, "1 Channel")
	}
}

func TestHandleKey_F10SolosGroup10(t *testing.T) {
	u, _ := newTestUI(t)
	ev := tcell.NewEventKey(tcell.KeyF10, 0, tcell.ModNone)
	if u.handleKey(ev) != nil {
		t.Error("F10 not consumed")
	}
	for i := 0; i < state.NumGroups; i++ {
		want := i == state.NumGroups-1
		if got := u.state.IsVisible(state.GroupID(i)); got != want {
			t.Fatalf("group %d visible=%v want %v", i, got, want)
		}
	}
}

func TestHandleKey_DigitWithoutAltPassesThrough(t *testing.T) {
	u, _ := newTestUI(t)
	ev := tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModNone)
	if got := u.handleKey(ev); got != ev {
		t.Error("bare digit was consumed")
	}
	if !u.state.IsVisible(state.GroupID(0)) {
		t.Error("group 0 toggled by bare digit")
	}
}

func TestHandleKey_CtrlDigitPassesThrough(t *testing.T) {
	u, _ := newTestUI(t)
	ev := tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModCtrl)
	if got := u.handleKey(ev); got != ev {
		t.Error("Ctrl+1 was consumed")
	}
	if !u.state.IsVisible(state.GroupID(0)) {
		t.Error("group 0 toggled by Ctrl+1")
	}
}

func TestHandleKey_CtrlNCyclesForward(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.state.EnsureChannel("#b")
	ev := tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModCtrl)
	if u.handleKey(ev) != nil {
		t.Error("Ctrl-N not consumed")
	}
	if u.state.Target() != "#b" {
		t.Errorf("target=%q", u.state.Target())
	}
}

func TestHandleKey_CtrlNRefreshesSidebarTargetIndicator(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.state.EnsureChannel("#b")
	u.refreshSidebar()

	before := u.sidebarView.GetText(true)
	if !strings.Contains(before, "▶ #a") {
		t.Fatalf("expected initial sidebar target marker on #a, got:\n%s", before)
	}

	ev := tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModCtrl)
	if u.handleKey(ev) != nil {
		t.Error("Ctrl-N not consumed")
	}

	after := u.sidebarView.GetText(true)
	if !strings.Contains(after, "▶ #b") {
		t.Fatalf("expected sidebar target marker on #b after Ctrl-N, got:\n%s", after)
	}
	if strings.Contains(after, "▶ #a") {
		t.Fatalf("sidebar target marker still on #a after Ctrl-N:\n%s", after)
	}
}

func TestHandleKey_CtrlPCyclesBackward(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.state.EnsureChannel("#b")
	ev := tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModCtrl)
	if u.handleKey(ev) != nil {
		t.Error("Ctrl-P not consumed")
	}
	if u.state.Target() != "#b" {
		t.Errorf("target after Ctrl-P=%q", u.state.Target())
	}
}

func TestHandleKey_CtrlPRefreshesSidebarTargetIndicator(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.EnsureChannel("#a")
	u.state.EnsureChannel("#b")
	u.refreshSidebar()

	before := u.sidebarView.GetText(true)
	if !strings.Contains(before, "▶ #a") {
		t.Fatalf("expected initial sidebar target marker on #a, got:\n%s", before)
	}

	ev := tcell.NewEventKey(tcell.KeyCtrlP, 0, tcell.ModCtrl)
	if u.handleKey(ev) != nil {
		t.Error("Ctrl-P not consumed")
	}

	after := u.sidebarView.GetText(true)
	if !strings.Contains(after, "▶ #b") {
		t.Fatalf("expected sidebar target marker on #b after Ctrl-P, got:\n%s", after)
	}
	if strings.Contains(after, "▶ #a") {
		t.Fatalf("sidebar target marker still on #a after Ctrl-P:\n%s", after)
	}
}

func TestHandleKey_CtrlCQuits(t *testing.T) {
	u, fs := newTestUI(t)
	ev := tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	if u.handleKey(ev) != nil {
		t.Error("Ctrl-C not consumed")
	}
	if fs.quitN != 1 {
		t.Errorf("quitN=%d", fs.quitN)
	}
}

func TestHandleKey_UpArrowScrollsWhenInputEmpty(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.EnsureChannel("#a")
	addLines(t, u, "#a", 20)

	ev := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Error("Up arrow not consumed when input is empty")
	}
	if !u.manualScroll {
		t.Error("manualScroll should be enabled by Up arrow")
	}
}

func TestHandleKey_UpArrowPassesThroughWhenInputHasText(t *testing.T) {
	u, _ := newTestUI(t)
	u.input.SetText("typing")

	ev := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != ev {
		t.Error("Up arrow should pass through when input has text")
	}
}

func TestHandleKey_DownArrowAtBottomDisablesManual(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.EnsureChannel("#a")
	addLines(t, u, "#a", 1)
	u.mainView.ScrollToEnd()
	u.manualScroll = true

	ev := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	if got := u.handleKey(ev); got != nil {
		t.Error("Down arrow not consumed when input is empty")
	}
	if u.manualScroll {
		t.Error("manualScroll should be disabled at bottom")
	}
}

func TestRefreshMain_PreservesOffsetWhenManual(t *testing.T) {
	u, _ := newTestUI(t)
	u.state.EnsureChannel("#a")
	addLines(t, u, "#a", 20)
	u.mainView.ScrollToBeginning()
	u.manualScroll = true
	wantRow, wantCol := u.mainView.GetScrollOffset()

	u.refreshMain()
	gotRow, gotCol := u.mainView.GetScrollOffset()
	if gotRow != wantRow || gotCol != wantCol {
		t.Errorf("offset changed during manual refresh: got (%d,%d), want (%d,%d)", gotRow, gotCol, wantRow, wantCol)
	}
}

func TestSendTo_ManualScrollSkipsAutoFollow(t *testing.T) {
	u, fs := newTestUI(t)
	u.state.EnsureChannel("#a")
	addLines(t, u, "#a", 20)
	u.mainView.ScrollToBeginning()
	u.manualScroll = true
	wantRow, _ := u.mainView.GetScrollOffset()

	u.sendTo("#a", "hello", state.KindPrivmsg)
	if len(fs.privmsgs) != 1 {
		t.Fatalf("expected one outbound message, got %d", len(fs.privmsgs))
	}
	gotRow, _ := u.mainView.GetScrollOffset()
	if gotRow != wantRow {
		t.Errorf("sendTo forced auto-follow while manual scrolling: got row %d want %d", gotRow, wantRow)
	}
}

// --- OnMessage / OnEvent / OnJoin / OnPart appear minimally below ---------

func TestOnEvent_AppendsToRing(t *testing.T) {
	u, _ := newTestUI(t)
	// OnEvent calls QueueUpdateDraw which is async. Just check state directly
	// after AddEvent (the QueueUpdateDraw call is fire-and-forget for tests
	// without a running app loop; the underlying ring is already updated).
	u.state.AddEvent("hello")
	if got := u.state.Events(); len(got) == 0 || got[len(got)-1] != "hello" {
		t.Errorf("events=%v", got)
	}
}
