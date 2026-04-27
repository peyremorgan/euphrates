package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"euphrates/internal/state"
)

// Sender is the outbound side of the IRC connection that the UI talks to.
// The IRC client implementation satisfies it; tests pass a fake.
type Sender interface {
	// Nick returns our current nickname (for local message echo).
	Nick() string
	// SendPrivmsg sends a regular PRIVMSG to target.
	SendPrivmsg(target, text string) error
	// SendAction sends a CTCP ACTION ("/me ...") to target.
	SendAction(target, text string) error
	// Quit closes the connection with the given reason.
	Quit(reason string)
}

// UI owns the tview widgets and routes events between state and Sender.
type UI struct {
	app    *tview.Application
	state  *state.State
	sender Sender

	statusView *tview.TextView
	mainView   *tview.TextView
	eventsView *tview.TextView
	promptView *tview.TextView
	input      *tview.InputField
	inputRow   *tview.Flex
	root       *tview.Flex
}

// New builds a UI bound to the given state and Sender.
func New(s *state.State, sender Sender) *UI {
	u := &UI{
		app:    tview.NewApplication(),
		state:  s,
		sender: sender,
	}
	u.buildLayout()
	u.bindKeys()
	u.RefreshAll()
	return u
}

// Run starts the tview event loop and blocks until the UI is stopped.
func (u *UI) Run() error {
	return u.app.SetRoot(u.root, true).EnableMouse(false).Run()
}

// Stop ends the tview event loop.
func (u *UI) Stop() { u.app.Stop() }

// --- public hooks called from the IRC goroutine -----------------------------

// OnMessage stores msg in state and updates the main pane if the message is
// in a currently-visible group. Safe to call from any goroutine.
func (u *UI) OnMessage(msg state.Message) {
	line, visible := u.state.AppendMessage(msg)
	u.app.QueueUpdateDraw(func() {
		if visible {
			_, _ = fmt.Fprintln(u.mainView, line)
			u.mainView.ScrollToEnd()
		}
	})
}

// OnEvent appends a line to the events ring and refreshes the events view.
// Safe to call from any goroutine.
func (u *UI) OnEvent(line string) {
	u.state.AddEvent(line)
	u.app.QueueUpdateDraw(u.refreshEvents)
}

// addEvent is the synchronous counterpart to OnEvent, intended for callers
// that already run on the tview event-loop goroutine (input handlers,
// command dispatch). It does not queue a redraw — tview will repaint
// naturally when the input handler returns.
func (u *UI) addEvent(line string) {
	u.state.AddEvent(line)
	u.refreshEvents()
}

// OnJoin records membership of a channel and refreshes the status / prompt.
func (u *UI) OnJoin(channel string) {
	u.state.JoinChannel(channel)
	u.app.QueueUpdateDraw(func() {
		u.refreshStatus()
		u.refreshPrompt()
	})
}

// OnPart removes a channel and refreshes the views (parted channel's
// historical messages are filtered out by RenderVisible).
func (u *UI) OnPart(channel string) {
	u.state.PartChannel(channel)
	u.app.QueueUpdateDraw(u.refreshAfterStructuralChange)
}

// --- layout & refresh -------------------------------------------------------

func (u *UI) buildLayout() {
	u.statusView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.mainView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true).
		SetWordWrap(true)
	u.eventsView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	u.promptView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)

	u.input = tview.NewInputField()
	u.input.SetDoneFunc(u.onInputDone)

	u.inputRow = tview.NewFlex().SetDirection(tview.FlexColumn)
	u.inputRow.AddItem(u.promptView, 0, 0, false)
	u.inputRow.AddItem(u.input, 0, 1, true)

	u.root = tview.NewFlex().SetDirection(tview.FlexRow)
	u.root.AddItem(u.statusView, 1, 0, false)
	u.root.AddItem(u.mainView, 0, 1, false)
	u.root.AddItem(u.eventsView, 5, 0, false)
	u.root.AddItem(u.inputRow, 1, 0, true)
}

// RefreshAll repaints every widget from current state. Called once at
// startup; not normally needed thereafter.
func (u *UI) RefreshAll() {
	u.refreshStatus()
	u.refreshMain()
	u.refreshEvents()
	u.refreshPrompt()
}

func (u *UI) refreshStatus() {
	u.statusView.SetText(FormatStatus(u.state))
}

func (u *UI) refreshMain() {
	u.mainView.Clear()
	for _, line := range u.state.RenderVisible() {
		_, _ = fmt.Fprintln(u.mainView, line)
	}
	u.mainView.ScrollToEnd()
}

func (u *UI) refreshEvents() {
	u.eventsView.Clear()
	for _, line := range u.state.Events() {
		_, _ = fmt.Fprintln(u.eventsView, line)
	}
}

func (u *UI) refreshPrompt() {
	text, width := FormatPrompt(u.state)
	u.promptView.SetText(text)
	u.inputRow.ResizeItem(u.promptView, width, 0)
}

// refreshAfterStructuralChange is the heavy refresh used after operations
// that may change which messages are visible (toggle, part).
func (u *UI) refreshAfterStructuralChange() {
	u.refreshStatus()
	u.refreshMain()
	u.refreshPrompt()
}

// --- input handling ---------------------------------------------------------

func (u *UI) bindKeys() {
	u.app.SetInputCapture(u.handleKey)
}

// handleKey is exported-shaped (CamelCase semantics) but kept lower-case
// because it's an internal capture. Returns nil to consume.
func (u *UI) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	// Alt+digit solos a numeric group.
	if ev.Modifiers()&tcell.ModAlt != 0 {
		if g, ok := groupForRune(ev.Rune()); ok {
			u.soloGroup(g)
			return nil
		}
	}
	// Ctrl+digit toggles a numeric group.
	if ev.Modifiers()&tcell.ModCtrl != 0 {
		if g, ok := groupForRune(ev.Rune()); ok {
			u.toggleGroup(g)
			return nil
		}
	}
	switch ev.Key() {
	case tcell.KeyCtrlN:
		u.state.NextChannel()
		u.refreshStatus()
		u.refreshPrompt()
		return nil
	case tcell.KeyCtrlP:
		u.state.PrevChannel()
		u.refreshStatus()
		u.refreshPrompt()
		return nil
	case tcell.KeyCtrlC:
		u.sender.Quit("euphrates closing")
		u.Stop()
		return nil
	}
	return ev
}

// groupForRune maps a digit rune to its GroupID. '1'..'9' -> 0..8, '0' -> 9.
func groupForRune(r rune) (state.GroupID, bool) {
	if r >= '1' && r <= '9' {
		return state.GroupID(r - '1'), true
	}
	if r == '0' {
		return state.GroupID(state.NumGroups - 1), true
	}
	return 0, false
}

func (u *UI) toggleGroup(g state.GroupID) {
	u.state.ToggleGroup(g)
	u.refreshAfterStructuralChange()
}

func (u *UI) soloGroup(g state.GroupID) {
	u.state.SoloNumericGroup(g)
	u.refreshAfterStructuralChange()
}

// onInputDone fires when the InputField completes. Enter submits; Esc clears.
func (u *UI) onInputDone(key tcell.Key) {
	switch key {
	case tcell.KeyEscape:
		u.input.SetText("")
	case tcell.KeyEnter:
		text := u.input.GetText()
		u.input.SetText("")
		u.handleSubmit(text)
	}
}

// handleSubmit interprets a submitted line as a command (`/me ...`,
// `/quit ...`) or a plain message to the current target. Pure-ish: relies on
// state and Sender but no tview surface, so it's directly testable.
func (u *UI) handleSubmit(text string) {
	text = strings.TrimRight(text, " \t")
	if text == "" {
		return
	}
	target := u.state.Target()
	if strings.HasPrefix(text, "/") {
		u.handleCommand(text)
		return
	}
	if target == "" {
		u.addEvent("(no target — join a channel first)")
		return
	}
	u.sendTo(target, text, state.KindPrivmsg)
}

func (u *UI) handleCommand(line string) {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToLower(parts[0])
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}
	switch cmd {
	case "/me":
		target := u.state.Target()
		if target == "" || rest == "" {
			u.addEvent("(usage: /me <action>)")
			return
		}
		u.sendTo(target, rest, state.KindAction)
	case "/quit":
		reason := rest
		if reason == "" {
			reason = "euphrates"
		}
		u.sender.Quit(reason)
		u.Stop()
	default:
		u.addEvent("(unknown command: " + cmd + ")")
	}
}

// sendTo routes an outbound line: forces target visibility, transmits it,
// and locally echoes it into the message log on success.
func (u *UI) sendTo(target, text string, kind state.MessageKind) {
	u.state.ForceTargetVisible()

	var err error
	switch kind {
	case state.KindAction:
		err = u.sender.SendAction(target, text)
	default:
		err = u.sender.SendPrivmsg(target, text)
	}
	if err != nil {
		u.addEvent("send failed: " + err.Error())
		return
	}

	echo := state.Message{
		Channel: target,
		Nick:    u.sender.Nick(),
		Text:    text,
		Kind:    kind,
	}
	line, visible := u.state.AppendMessage(echo)
	// ForceTargetVisible above ensures `visible` is true, but we keep the
	// guard to defend against future refactors.
	if visible {
		_, _ = fmt.Fprintln(u.mainView, line)
		u.mainView.ScrollToEnd()
	}
	// The forced-visibility may have flipped a group on; refresh the
	// indicators so the user can see what changed.
	u.refreshStatus()
	u.refreshPrompt()
}
