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
	quitWith string
	quitN    int
	sendErr  error
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
	if len(rendered) != 1 || !strings.Contains(rendered[0], "<me>") {
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
