package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

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
	// Join asks the server to join channel.
	Join(channel string) error
	// Part asks the server to leave channel with an optional reason.
	Part(channel, reason string) error
}

// UI owns the tview widgets and routes events between state and Sender.
type UI struct {
	app    *tview.Application
	state  *state.State
	sender Sender

	manualScroll bool

	commandCompletion commandCompletionState
	joinCompletion    joinCompletionState
	partCompletion    partCompletionState

	statusView       *tview.TextView
	statusCountView  *tview.TextView
	statusRow        *tview.Flex
	sidebarView      *tview.TextView
	sidebarTitleView *tview.TextView
	sidebarCol       *tview.Flex
	sidebarDivider   *tview.TextView
	usersView        *tview.TextView
	usersTitleView   *tview.TextView
	usersCol         *tview.Flex
	usersDivider     *tview.TextView
	contentRow       *tview.Flex
	mainView         *tview.TextView
	separatorView    *tview.TextView
	eventsView       *tview.TextView
	promptView       *tview.TextView
	input            *tview.InputField
	inputRow         *tview.Flex
	root             *tview.Flex
	pages            *tview.Pages
	browser          *channelBrowser
	sidebarVisible   bool
	usersVisible     bool
	browserVisible   bool
}

type joinCompletionState struct {
	expandedInput string
	matches       []string
}

type partCompletionState struct {
	expandedInput string
	matches       []string
}

type commandCompletionState struct {
	expandedInput string
	matches       []string
}

var knownCommands = []string{
	"/help",
	"/join",
	"/list",
	"/me",
	"/part",
	"/quit",
}

const dottedSeparatorRune = "┄"
const sidebarDividerRune = "│"
const maxEventsViewRows = 5
const sidebarWidth = 30
const usersPanelWidth = 30

// New builds a UI bound to the given state and Sender.
func New(s *state.State, sender Sender) *UI {
	u := &UI{
		app:    tview.NewApplication(),
		state:  s,
		sender: sender,
	}
	u.buildLayout()
	u.app.SetBeforeDrawFunc(func(tcell.Screen) bool {
		u.refreshSidebarDivider()
		u.refreshUsersDivider()
		u.refreshSeparator()
		return false
	})
	u.bindKeys()
	u.RefreshAll()
	return u
}

// Run starts the tview event loop and blocks until the UI is stopped.
func (u *UI) Run() error {
	return u.app.SetRoot(u.pages, true).EnableMouse(false).Run()
}

// Stop ends the tview event loop.
func (u *UI) Stop() { u.app.Stop() }

// --- public hooks called from the IRC goroutine -----------------------------

// OnMessage stores msg in state and updates the main pane if the message is
// in a currently-visible group. Safe to call from any goroutine.
func (u *UI) OnMessage(msg state.Message) {
	line, visible := u.state.AppendMessage(msg)
	u.app.QueueUpdateDraw(func() {
		u.refreshUsersPanel()
		if visible {
			_, _ = fmt.Fprintln(u.mainView, line)
			if !u.manualScroll {
				u.mainView.ScrollToEnd()
			}
		}
	})
}

// OnEvent appends a line to the events ring and refreshes the events view.
// Safe to call from any goroutine.
func (u *UI) OnEvent(line string) {
	u.state.AddEvent(line)
	u.app.QueueUpdateDraw(func() {
		u.refreshEvents()
		u.refreshUsersPanel()
	})
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
		u.refreshSidebar()
		u.refreshUsersPanel()
		u.refreshPrompt()
	})
}

// OnPart removes a channel and refreshes the views (parted channel's
// historical messages are filtered out by RenderVisible).
func (u *UI) OnPart(channel string) {
	u.state.PartChannel(channel)
	u.app.QueueUpdateDraw(u.refreshAfterStructuralChange)
}

// OnNames replaces the known user roster for channel.
func (u *UI) OnNames(channel string, nicks []string) {
	u.state.SetChannelUsers(channel, nicks)
	u.app.QueueUpdateDraw(u.refreshUsersPanel)
}

// OnUserJoin marks nick present in channel.
func (u *UI) OnUserJoin(channel, nick string) {
	u.state.AddUserToChannel(channel, nick)
	u.app.QueueUpdateDraw(u.refreshUsersPanel)
}

// OnUserPart removes nick from one channel.
func (u *UI) OnUserPart(channel, nick string) {
	u.state.RemoveUserFromChannel(channel, nick)
	u.app.QueueUpdateDraw(u.refreshUsersPanel)
}

// OnUserQuit removes nick from all channels.
func (u *UI) OnUserQuit(nick string) {
	u.state.RemoveUserFromAllChannels(nick)
	u.app.QueueUpdateDraw(u.refreshUsersPanel)
}

// OnUserNick renames nick across tracked channels.
func (u *UI) OnUserNick(oldNick, newNick string) {
	u.state.RenameUserInAllChannels(oldNick, newNick)
	u.app.QueueUpdateDraw(u.refreshUsersPanel)
}

// --- layout & refresh -------------------------------------------------------

func (u *UI) buildLayout() {
	chrome := state.ActiveChromeTheme()

	u.statusView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.statusView.SetBackgroundColor(tcell.GetColor(chrome.StatusBackground))
	u.statusView.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.StatusBackground)),
	)
	u.statusCountView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignRight)
	u.statusCountView.SetBackgroundColor(tcell.GetColor(chrome.StatusBackground))
	u.statusCountView.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.StatusBackground)),
	)
	u.statusRow = tview.NewFlex().SetDirection(tview.FlexColumn)
	u.statusRow.AddItem(u.statusView, 0, 1, false)
	u.statusRow.AddItem(u.statusCountView, 0, 0, false)
	u.sidebarView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	u.sidebarView.SetTextColor(tcell.GetColor(chrome.EventsForeground))

	u.sidebarTitleView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.sidebarTitleView.SetBackgroundColor(tcell.GetColor(chrome.Separator))
	u.sidebarTitleView.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.Separator)),
	)
	u.sidebarTitleView.SetText(" Groups ")

	u.sidebarCol = tview.NewFlex().SetDirection(tview.FlexRow)
	u.sidebarCol.AddItem(u.sidebarTitleView, 1, 0, false)
	u.sidebarCol.AddItem(u.sidebarView, 0, 1, false)

	u.sidebarDivider = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.usersDivider = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.usersView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	u.usersView.SetTextColor(tcell.GetColor(chrome.EventsForeground))
	u.usersTitleView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.usersTitleView.SetBackgroundColor(tcell.GetColor(chrome.Separator))
	u.usersTitleView.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.Separator)),
	)
	u.usersTitleView.SetText(" Users ")
	u.usersCol = tview.NewFlex().SetDirection(tview.FlexRow)
	u.usersCol.AddItem(u.usersTitleView, 1, 0, false)
	u.usersCol.AddItem(u.usersView, 0, 1, false)
	u.mainView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true).
		SetWordWrap(true)
	u.contentRow = tview.NewFlex().SetDirection(tview.FlexColumn)
	u.contentRow.AddItem(u.sidebarCol, 0, 0, false)
	u.contentRow.AddItem(u.sidebarDivider, 0, 0, false)
	u.contentRow.AddItem(u.mainView, 0, 1, false)
	u.contentRow.AddItem(u.usersDivider, 0, 0, false)
	u.contentRow.AddItem(u.usersCol, 0, 0, false)
	u.separatorView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.eventsView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true)
	u.eventsView.SetTextColor(tcell.GetColor(chrome.EventsForeground))
	u.promptView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	u.promptView.SetTextColor(tcell.GetColor(chrome.PromptForeground))

	u.input = tview.NewInputField()
	u.input.SetDoneFunc(u.onInputDone)

	u.inputRow = tview.NewFlex().SetDirection(tview.FlexColumn)
	u.inputRow.AddItem(u.promptView, 0, 0, false)
	u.inputRow.AddItem(u.input, 0, 1, true)

	u.root = tview.NewFlex().SetDirection(tview.FlexRow)
	u.root.AddItem(u.statusRow, 1, 0, false)
	u.root.AddItem(u.contentRow, 0, 1, false)
	u.root.AddItem(u.separatorView, 1, 0, false)
	u.root.AddItem(u.eventsView, 5, 0, false)
	u.root.AddItem(u.inputRow, 1, 0, true)

	u.browser = newChannelBrowser()
	u.pages = tview.NewPages()
	u.pages.AddPage("main", u.root, true, true)
	u.pages.AddPage("browser", centerPrimitive(u.browser.root, browserModalWidth, browserModalHeight), true, false)
}

// RefreshAll repaints every widget from current state. Called once at
// startup; not normally needed thereafter.
func (u *UI) RefreshAll() {
	u.refreshStatus()
	u.refreshSidebar()
	u.refreshUsersPanel()
	u.refreshMain()
	u.refreshSidebarDivider()
	u.refreshUsersDivider()
	u.refreshSeparator()
	u.refreshEvents()
	u.refreshPrompt()
}

func (u *UI) refreshStatus() {
	u.statusView.SetText(FormatStatus(u.state))
	countText := FormatStatusCount(u.state)
	u.statusCountView.SetText(countText)
	u.statusRow.ResizeItem(u.statusCountView, tview.TaggedStringWidth(countText), 0)
}

func (u *UI) refreshMain() {
	row, col := u.mainView.GetScrollOffset()
	u.mainView.Clear()
	for _, line := range u.state.RenderVisible() {
		_, _ = fmt.Fprintln(u.mainView, line)
	}
	if u.manualScroll {
		u.mainView.ScrollTo(row, col)
		return
	}
	u.mainView.ScrollToEnd()
}

func (u *UI) refreshEvents() {
	events := u.state.Events()
	u.root.ResizeItem(u.eventsView, eventsViewHeight(len(events)), 0)
	u.eventsView.Clear()
	for _, line := range events {
		_, _ = fmt.Fprintln(u.eventsView, line)
	}
}

func (u *UI) refreshSidebar() {
	channels := u.state.Channels()
	target := u.state.Target()
	grouped := make([][]string, state.NumGroups)
	for _, c := range channels {
		if !c.Group.IsNumeric() {
			continue
		}
		grouped[int(c.Group)] = append(grouped[int(c.Group)], c.Name)
	}

	var b strings.Builder
	for i := 0; i < state.NumGroups; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(formatGroupHeader(i, u.state.IsVisible(state.GroupID(i))))
		for _, name := range grouped[i] {
			b.WriteByte('\n')
			if target != "" && strings.EqualFold(name, target) {
				b.WriteString("▶ ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString(state.Escape(name))
		}
	}
	u.sidebarView.SetText(b.String())
}

func (u *UI) refreshUsersPanel() {
	users := u.state.UsersSortedByActivity()
	targetUsers := u.state.UsersInChannel(u.state.Target())
	u.usersTitleView.SetText(" Users — " + formatDecimalSI(len(users)) + " ")

	var b strings.Builder
	for i, user := range users {
		if i > 0 {
			b.WriteByte('\n')
		}
		if targetUsers[user.Key] {
			b.WriteString("• ")
		} else {
			b.WriteString("  ")
		}
		b.WriteString(state.UserColor(user.Nick))
		b.WriteString(state.Escape(user.Nick))
		b.WriteString(state.ResetColor())
	}
	u.usersView.SetText(b.String())
}

func formatDecimalSI(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	units := []string{"k", "M", "G", "T", "P", "E"}
	value := float64(n)
	unit := 0
	for value >= 1000 && unit < len(units)-1 {
		value /= 1000
		unit++
	}
	idx := unit - 1
	if idx < 0 {
		idx = 0
	}

	if value >= 10 {
		rounded := math.Round(value)
		if rounded >= 1000 && idx < len(units)-1 {
			rounded = 1
			idx++
		}
		return fmt.Sprintf("%.0f%s", rounded, units[idx])
	}

	rounded := math.Round(value*10) / 10
	if rounded >= 1000 && idx < len(units)-1 {
		rounded = 1
		idx++
	}
	if rounded >= 10 {
		return fmt.Sprintf("%.0f%s", rounded, units[idx])
	}
	txt := fmt.Sprintf("%.1f", rounded)
	txt = strings.TrimSuffix(txt, ".0")
	return txt + units[idx]
}

func formatGroupHeader(groupIndex int, visible bool) string {
	digit := digitForGroup(groupIndex)
	// Create a left-aligned header like: ● 1 ─────────────────────────
	const lineChar = "─"
	const totalWidth = sidebarWidth
	const indicatorVisible = "●"
	const indicatorHidden = "○"
	const indicatorWidth = 2 // circle + trailing space
	const digitWidth = 1
	const gapWidth = 1 // spacing between digit and line

	lineWidth := totalWidth - indicatorWidth - digitWidth - gapWidth
	if lineWidth < 1 {
		lineWidth = 1
	}

	indicator := indicatorHidden
	if visible {
		indicator = indicatorVisible
	}

	return indicator + " " + digit + " " + strings.Repeat(lineChar, lineWidth)
}

func eventsViewHeight(lines int) int {
	if lines <= 0 {
		return 0
	}
	if lines > maxEventsViewRows {
		return maxEventsViewRows
	}
	return lines
}

func (u *UI) refreshSidebarDivider() {
	if u.sidebarDivider == nil || !u.sidebarVisible {
		if u.sidebarDivider != nil {
			u.sidebarDivider.SetText("")
		}
		return
	}
	_, _, _, height := u.sidebarDivider.GetRect()
	if height <= 0 {
		_, _, _, height = u.contentRow.GetRect()
	}
	if height <= 0 {
		u.sidebarDivider.SetText("")
		return
	}

	var b strings.Builder
	for i := 0; i < height; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sidebarDividerRune)
	}
	sepColor := state.ActiveChromeTheme().Separator
	u.sidebarDivider.SetText("[" + sepColor + "]" + b.String() + state.ResetColor())
}

func (u *UI) refreshUsersDivider() {
	if u.usersDivider == nil || !u.usersVisible {
		if u.usersDivider != nil {
			u.usersDivider.SetText("")
		}
		return
	}
	_, _, _, height := u.usersDivider.GetRect()
	if height <= 0 {
		_, _, _, height = u.contentRow.GetRect()
	}
	if height <= 0 {
		u.usersDivider.SetText("")
		return
	}

	var b strings.Builder
	for i := 0; i < height; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sidebarDividerRune)
	}
	sepColor := state.ActiveChromeTheme().Separator
	u.usersDivider.SetText("[" + sepColor + "]" + b.String() + state.ResetColor())
}

func (u *UI) refreshSeparator() {
	if u.separatorView == nil {
		return
	}
	_, _, width, _ := u.separatorView.GetRect()
	if width <= 0 {
		_, _, width, _ = u.mainView.GetRect()
	}
	sep := dottedSeparator(width)
	if sep == "" {
		u.separatorView.SetText("")
		return
	}
	sepColor := state.ActiveChromeTheme().Separator
	u.separatorView.SetText("[" + sepColor + "]" + sep + state.ResetColor())
}

func dottedSeparator(width int) string {
	if width <= 0 {
		return ""
	}
	return strings.Repeat(dottedSeparatorRune, width)
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
	u.refreshSidebar()
	u.refreshUsersPanel()
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
	if u.browserVisible {
		if ev.Modifiers()&tcell.ModAlt != 0 && unicode.ToLower(ev.Rune()) == 'l' {
			u.closeBrowser()
			return nil
		}
		if ev.Key() == tcell.KeyEscape {
			u.closeBrowser()
			return nil
		}
		if ev.Key() == tcell.KeyEnter {
			u.joinSelectedBrowserChannel()
			return nil
		}
		if ev.Modifiers()&tcell.ModAlt != 0 && unicode.ToLower(ev.Rune()) == 'j' {
			u.joinSelectedBrowserChannel()
			return nil
		}
		if u.browser.handleKey(ev) {
			return nil
		}
		return nil
	}

	// Alt+digit toggles a numeric group.
	if ev.Modifiers()&tcell.ModAlt != 0 {
		if unicode.ToLower(ev.Rune()) == 'g' {
			u.toggleSidebar()
			return nil
		}
		if unicode.ToLower(ev.Rune()) == 'u' {
			u.toggleUsersPanel()
			return nil
		}
		if unicode.ToLower(ev.Rune()) == 'l' {
			u.toggleBrowser()
			return nil
		}
		if g, ok := groupForRune(ev.Rune()); ok {
			u.toggleGroup(g)
			return nil
		}
	}
	switch ev.Key() {
	case tcell.KeyF1, tcell.KeyF2, tcell.KeyF3, tcell.KeyF4, tcell.KeyF5,
		tcell.KeyF6, tcell.KeyF7, tcell.KeyF8, tcell.KeyF9, tcell.KeyF10:
		if g, ok := groupForFunctionKey(ev.Key()); ok {
			u.soloGroup(g)
			return nil
		}
	case tcell.KeyUp:
		if ev.Modifiers() == tcell.ModNone && u.input.GetText() == "" {
			u.scrollMainUp()
			return nil
		}
	case tcell.KeyDown:
		if ev.Modifiers() == tcell.ModNone && u.input.GetText() == "" {
			u.scrollMainDown()
			return nil
		}
	case tcell.KeyCtrlN:
		u.state.NextChannel()
		u.refreshStatus()
		u.refreshSidebar()
		u.refreshUsersPanel()
		u.refreshPrompt()
		return nil
	case tcell.KeyCtrlP:
		u.state.PrevChannel()
		u.refreshStatus()
		u.refreshSidebar()
		u.refreshUsersPanel()
		u.refreshPrompt()
		return nil
	case tcell.KeyCtrlC:
		u.sender.Quit("euphrates closing")
		u.Stop()
		return nil
	case tcell.KeyTab:
		if u.tryCommandCompletion() {
			return nil
		}
		if u.tryJoinCompletion() {
			return nil
		}
		if u.tryPartCompletion() {
			return nil
		}
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

// groupForFunctionKey maps F1..F10 -> 0..9.
func groupForFunctionKey(k tcell.Key) (state.GroupID, bool) {
	if k >= tcell.KeyF1 && k <= tcell.KeyF9 {
		return state.GroupID(k - tcell.KeyF1), true
	}
	if k == tcell.KeyF10 {
		return state.GroupID(state.NumGroups - 1), true
	}
	return 0, false
}

func (u *UI) toggleGroup(g state.GroupID) {
	u.state.ToggleGroup(g)
	u.refreshAfterStructuralChange()
}

func (u *UI) toggleSidebar() {
	u.sidebarVisible = !u.sidebarVisible
	if u.sidebarVisible {
		u.refreshSidebar()
		u.refreshSidebarDivider()
		u.contentRow.ResizeItem(u.sidebarCol, sidebarWidth, 0)
		u.contentRow.ResizeItem(u.sidebarDivider, 1, 0)
		return
	}
	u.contentRow.ResizeItem(u.sidebarCol, 0, 0)
	u.contentRow.ResizeItem(u.sidebarDivider, 0, 0)
	u.sidebarDivider.SetText("")
}

func (u *UI) toggleUsersPanel() {
	u.usersVisible = !u.usersVisible
	if u.usersVisible {
		u.refreshUsersPanel()
		u.refreshUsersDivider()
		u.contentRow.ResizeItem(u.usersDivider, 1, 0)
		u.contentRow.ResizeItem(u.usersCol, usersPanelWidth, 0)
		return
	}
	u.contentRow.ResizeItem(u.usersDivider, 0, 0)
	u.contentRow.ResizeItem(u.usersCol, 0, 0)
	u.usersDivider.SetText("")
}

func (u *UI) soloGroup(g state.GroupID) {
	u.state.SoloNumericGroup(g)
	u.refreshAfterStructuralChange()
}

func (u *UI) scrollMainUp() {
	row, col := u.mainView.GetScrollOffset()
	if row > 0 {
		u.mainView.ScrollTo(row-1, col)
	}
	u.manualScroll = true
}

func (u *UI) scrollMainDown() {
	if u.mainAtBottom() {
		u.manualScroll = false
		u.mainView.ScrollToEnd()
		return
	}
	row, col := u.mainView.GetScrollOffset()
	u.mainView.ScrollTo(row+1, col)
	u.manualScroll = !u.mainAtBottom()
	if !u.manualScroll {
		u.mainView.ScrollToEnd()
	}
}

func (u *UI) toggleBrowser() {
	if u.browserVisible {
		u.closeBrowser()
		return
	}
	u.openBrowser()
}

func (u *UI) openBrowser() {
	channels := u.state.ListCachedChannels()
	u.browser.reset()
	u.browser.setChannels(channels, u.state.IsJoinedNormalChannel)
	u.browserVisible = true
	u.pages.ShowPage("browser")
	u.pages.SendToFront("browser")
	u.app.SetFocus(u.browser.root)
}

func (u *UI) closeBrowser() {
	if !u.browserVisible {
		return
	}
	u.browserVisible = false
	u.pages.HidePage("browser")
	u.app.SetFocus(u.input)
}

func (u *UI) joinSelectedBrowserChannel() {
	channel := u.browser.selectedChannel()
	if channel == "" {
		return
	}
	if err := u.sender.Join(channel); err != nil {
		u.addEvent("join failed: " + err.Error())
		return
	}
	u.closeBrowser()
}

func (u *UI) mainAtBottom() bool {
	row, _ := u.mainView.GetScrollOffset()
	_, _, _, h := u.mainView.GetRect()
	if h <= 0 {
		h = 1
	}
	maxRow := u.mainView.GetWrappedLineCount() - h
	if maxRow < 0 {
		maxRow = 0
	}
	return row >= maxRow
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

// handleSubmit interprets a submitted line as a command (`/help`, `/me ...`,
// `/join ...`, `/part ...`, `/quit ...`) or a plain message to the current
// target. Pure-ish: relies on
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
	case "/help":
		u.addEvent("commands: /help /join /list /me /part /quit")
		u.addEvent("usage: /join <channel> | /list | /part [channel] [reason] | /me <action> | /quit [reason]")
	case "/list":
		u.openBrowser()
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
			reason = "Bye!"
		}
		u.sender.Quit(reason)
		u.Stop()
	case "/join":
		if rest == "" {
			u.addEvent("(usage: /join <channel>)")
			return
		}
		if err := u.sender.Join(rest); err != nil {
			u.addEvent("join failed: " + err.Error())
		}
	case "/part":
		target := ""
		reason := ""

		if rest == "" {
			target = u.state.Target()
		} else {
			first, tail := splitFirstArg(rest)
			if c, ok := u.state.Channel(first); ok {
				target = c.Name
				reason = tail
			} else if strings.EqualFold(first, state.ServerChannelName) {
				target = state.ServerChannelName
				reason = tail
			} else if isChannelName(first) {
				target = first
				reason = tail
			} else {
				target = u.state.Target()
				reason = strings.TrimSpace(rest)
			}
		}

		if target == "" {
			u.addEvent("(no target — join a channel first)")
			return
		}
		if c, ok := u.state.Channel(target); ok {
			if c.Kind != state.ChanNormal {
				u.addEvent("(cannot part from queries/server)")
				return
			}
			target = c.Name
		} else if !isChannelName(target) {
			u.addEvent("(cannot part from queries/server)")
			return
		}

		if err := u.sender.Part(target, reason); err != nil {
			u.addEvent("part failed: " + err.Error())
		}
	default:
		u.addEvent("(unknown command: " + cmd + ")")
	}
}

func splitFirstArg(s string) (first, rest string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeft(s[i+1:], " \t")
}

func isChannelName(name string) bool {
	if name == "" {
		return false
	}
	switch name[0] {
	case '#', '&', '+', '!':
		return true
	default:
		return false
	}
}

func (u *UI) tryJoinCompletion() bool {
	text := u.input.GetText()
	if !strings.HasPrefix(strings.ToLower(text), "/join ") {
		u.joinCompletion = joinCompletionState{}
		return false
	}
	if strings.EqualFold(text, "/join ") {
		u.joinCompletion = joinCompletionState{}
		u.input.SetText("/join #")
		return true
	}
	if text == u.joinCompletion.expandedInput && len(u.joinCompletion.matches) > 1 {
		u.showCompletionList(u.joinCompletion.matches)
		return true
	}
	u.joinCompletion = joinCompletionState{}

	partial := text[len("/join "):]
	matches := u.state.MatchJoinableChannels(partial)
	if len(matches) == 0 {
		return true
	}
	if len(matches) == 1 {
		u.input.SetText("/join " + matches[0])
		return true
	}

	lcp := longestCommonPrefix(matches)
	if lcp != "" && !strings.EqualFold(lcp, partial) {
		expanded := "/join " + lcp
		u.input.SetText(expanded)
		u.joinCompletion = joinCompletionState{
			expandedInput: expanded,
			matches:       append([]string(nil), matches...),
		}
		return true
	}

	u.showCompletionList(matches)
	return true
}

func (u *UI) tryPartCompletion() bool {
	text := u.input.GetText()
	if !strings.HasPrefix(strings.ToLower(text), "/part ") {
		u.partCompletion = partCompletionState{}
		return false
	}
	if text == u.partCompletion.expandedInput && len(u.partCompletion.matches) > 1 {
		u.showCompletionList(u.partCompletion.matches)
		return true
	}
	u.partCompletion = partCompletionState{}

	partial := text[len("/part "):]
	matches := u.state.MatchPartableChannels(partial)
	if len(matches) == 0 {
		return true
	}
	if len(matches) == 1 {
		u.input.SetText("/part " + matches[0])
		return true
	}

	lcp := longestCommonPrefix(matches)
	if lcp != "" && !strings.EqualFold(lcp, partial) {
		expanded := "/part " + lcp
		u.input.SetText(expanded)
		u.partCompletion = partCompletionState{
			expandedInput: expanded,
			matches:       append([]string(nil), matches...),
		}
		return true
	}

	u.showCompletionList(matches)
	return true
}

func (u *UI) tryCommandCompletion() bool {
	text := u.input.GetText()
	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \t") {
		u.commandCompletion = commandCompletionState{}
		return false
	}
	if text == u.commandCompletion.expandedInput && len(u.commandCompletion.matches) > 1 {
		u.showCompletionList(u.commandCompletion.matches)
		return true
	}
	u.commandCompletion = commandCompletionState{}

	matches := make([]string, 0, len(knownCommands))
	for _, cmd := range knownCommands {
		if strings.HasPrefix(strings.ToLower(cmd), strings.ToLower(text)) {
			matches = append(matches, cmd)
		}
	}
	if len(matches) == 0 {
		return true
	}
	if len(matches) == 1 {
		u.input.SetText(matches[0] + " ")
		return true
	}

	lcp := longestCommonPrefix(matches)
	if lcp != "" && !strings.EqualFold(lcp, text) {
		u.input.SetText(lcp)
		u.commandCompletion = commandCompletionState{
			expandedInput: lcp,
			matches:       append([]string(nil), matches...),
		}
		return true
	}

	u.showCompletionList(matches)
	return true
}

func (u *UI) showCompletionList(matches []string) {
	if len(matches) == 0 {
		return
	}
	_, _, w, _ := u.eventsView.GetRect()
	for _, line := range formatCompletionLines(matches, w) {
		u.addEvent(line)
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
		Time:    time.Now(),
	}
	line, visible := u.state.AppendMessage(echo)
	// ForceTargetVisible above ensures `visible` is true, but we keep the
	// guard to defend against future refactors.
	if visible {
		_, _ = fmt.Fprintln(u.mainView, line)
		if !u.manualScroll {
			u.mainView.ScrollToEnd()
		}
	}
	// The forced-visibility may have flipped a group on; refresh the
	// indicators so the user can see what changed.
	u.refreshStatus()
	u.refreshPrompt()
}
