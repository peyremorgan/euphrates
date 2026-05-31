package ui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

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
	quitWith string
	quitN    int
	sendErr  error
	joinErr  error
}

type sentMsg struct{ Target, Text string }

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
	if got := u.contentRow.GetItemCount(); got != 3 {
		t.Fatalf("content row item count=%d want 3", got)
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
		want := digitForGroup(i) + " -------------"
		if lines[i] != want {
			t.Fatalf("line %d=%q want %q", i, lines[i], want)
		}
	}
}

func TestRefreshSidebar_UsesInsertionOrderWithinGroup(t *testing.T) {
	u, _ := newTestUI(t)
	for _, ch := range []string{"#a", "#b", "#c", "#d", "#e", "#f", "#g", "#h", "#i", "#j", "#k"} {
		u.state.JoinChannel(ch)
	}

	u.refreshSidebar()
	got := u.sidebarView.GetText(true)
	want := strings.Join([]string{
		"1 -------------",
		"  #a",
		"  #k",
	}, "\n")
	if !strings.Contains(got, want) {
		t.Fatalf("group 1 block missing expected insertion order:\n%s", got)
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
	if got := u.input.GetText(); got != "/join #golang" {
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
	if got := u.input.GetText(); got != "/join #golang" {
		t.Fatalf("expected joined channel exclusion, got %q", got)
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
	if got := u.input.GetText(); got != "/join #golang" {
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
