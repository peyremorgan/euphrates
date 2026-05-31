package ui

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"euphrates/internal/state"
)

const (
	browserModalWidth  = 72
	browserModalHeight = 20
)

const browserJoinMarker = "* "
const browserPlainMarker = "  "

type channelBrowser struct {
	root        *tview.Flex
	titleView   *tview.TextView
	searchView  *tview.TextView
	listView    *tview.List
	statusLeft  *tview.TextView
	statusRight *tview.TextView
	statusRow   *tview.Flex

	allChannels      []string
	filteredChannels []string
	searchQuery      string

	isJoined func(string) bool
}

func newChannelBrowser() *channelBrowser {
	b := &channelBrowser{}
	chrome := state.ActiveChromeTheme()

	b.titleView = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	b.titleView.SetBackgroundColor(tcell.GetColor(chrome.StatusBackground))
	b.titleView.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.StatusBackground)),
	)

	b.searchView = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	b.searchView.SetBackgroundColor(tcell.GetColor(chrome.Separator))
	b.searchView.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.Separator)),
	)

	b.listView = tview.NewList().
		ShowSecondaryText(false).
		SetWrapAround(false).
		SetMainTextColor(tcell.GetColor(chrome.EventsForeground)).
		SetSelectedTextColor(tcell.GetColor(chrome.StatusForeground)).
		SetSelectedBackgroundColor(tcell.GetColor(chrome.StatusBackground))

	b.statusLeft = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	b.statusRight = tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetTextAlign(tview.AlignRight)
	b.statusLeft.SetBackgroundColor(tcell.GetColor(chrome.Separator))
	b.statusRight.SetBackgroundColor(tcell.GetColor(chrome.Separator))
	b.statusLeft.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.Separator)),
	)
	b.statusRight.SetTextStyle(
		tcell.StyleDefault.
			Foreground(tcell.GetColor(chrome.StatusForeground)).
			Background(tcell.GetColor(chrome.Separator)),
	)

	b.statusRow = tview.NewFlex().SetDirection(tview.FlexColumn)
	b.statusRow.AddItem(b.statusLeft, 0, 1, false)
	b.statusRow.AddItem(b.statusRight, 0, 0, false)

	b.root = tview.NewFlex().SetDirection(tview.FlexRow)
	b.root.SetBorder(true).SetTitle(" Channel browser ")
	b.root.AddItem(b.titleView, 1, 0, false)
	b.root.AddItem(b.searchView, 1, 0, false)
	b.root.AddItem(b.listView, 0, 1, false)
	b.root.AddItem(b.statusRow, 1, 0, false)

	b.listView.SetChangedFunc(func(_ int, _, _ string, _ rune) {
		b.refreshStatus()
	})

	b.reset()
	return b
}

func centerPrimitive(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewGrid().
		SetRows(0, height, 0).
		SetColumns(0, width, 0).
		AddItem(p, 1, 1, 1, 1, 0, 0, true)
}

func (b *channelBrowser) reset() {
	b.searchQuery = ""
	b.allChannels = nil
	b.filteredChannels = nil
	b.refreshTitle()
	b.refreshSearch()
	b.refreshList()
	b.refreshStatus()
}

func (b *channelBrowser) setChannels(channels []string, isJoined func(string) bool) {
	b.isJoined = isJoined
	b.allChannels = append([]string(nil), channels...)
	b.filteredChannels = append([]string(nil), channels...)
	b.listView.SetCurrentItem(0)
	b.refreshTitle()
	b.refreshSearch()
	b.refreshList()
	b.refreshStatus()
}

func (b *channelBrowser) appendRune(r rune) {
	oldLen := utf8.RuneCountInString(b.searchQuery)
	b.searchQuery += string(r)
	newLen := utf8.RuneCountInString(b.searchQuery)
	b.applyFilter(newLen > oldLen)
}

func (b *channelBrowser) backspace() {
	if b.searchQuery == "" {
		return
	}
	_, size := utf8.DecodeLastRuneInString(b.searchQuery)
	b.searchQuery = b.searchQuery[:len(b.searchQuery)-size]
	b.applyFilter(false)
}

func (b *channelBrowser) applyFilter(resetCursor bool) {
	selected := b.listView.GetCurrentItem()
	if selected < 0 {
		selected = 0
	}

	if b.searchQuery == "" {
		b.filteredChannels = append([]string(nil), b.allChannels...)
	} else {
		b.filteredChannels = sortChannelsByLCS(b.allChannels, b.searchQuery)
	}

	nextSelected := selected
	if resetCursor {
		nextSelected = 0
	} else {
		if nextSelected >= len(b.filteredChannels) {
			nextSelected = len(b.filteredChannels) - 1
		}
		if nextSelected < 0 {
			nextSelected = 0
		}
	}

	b.refreshSearch()
	b.refreshList()
	b.listView.SetCurrentItem(nextSelected)
	b.refreshStatus()
}

func (b *channelBrowser) refreshTitle() {
	b.titleView.SetText(" Channel browser ")
}

func (b *channelBrowser) refreshSearch() {
	query := b.searchQuery
	if query == "" {
		query = "(type to filter)"
	}
	b.searchView.SetText(" filter: " + state.Escape(query))
}

func (b *channelBrowser) listLabel(name string) string {
	prefix := browserPlainMarker
	if b.isJoined != nil && b.isJoined(name) {
		prefix = browserJoinMarker
	}
	return prefix + name
}

func (b *channelBrowser) refreshList() {
	b.listView.Clear()
	for _, name := range b.filteredChannels {
		b.listView.AddItem(b.listLabel(name), "", 0, nil)
	}
}

func (b *channelBrowser) refreshStatus() {
	b.statusLeft.SetText("Esc: quit    Alt-J: join channel")

	total := len(b.filteredChannels)
	if total == 0 {
		b.statusRight.SetText("0/0    0%")
		return
	}
	idx := b.listView.GetCurrentItem()
	if idx < 0 {
		idx = 0
	}
	if idx >= total {
		idx = total - 1
	}
	pct := int(math.Round(float64(idx+1) * 100 / float64(total)))
	b.statusRight.SetText(fmt.Sprintf("%d/%d    %d%%", idx+1, total, pct))
}

func (b *channelBrowser) selectedChannel() string {
	if len(b.filteredChannels) == 0 {
		return ""
	}
	idx := b.listView.GetCurrentItem()
	if idx < 0 || idx >= len(b.filteredChannels) {
		return ""
	}
	return b.filteredChannels[idx]
}

func (b *channelBrowser) moveSelection(delta int) {
	n := len(b.filteredChannels)
	if n == 0 {
		b.listView.SetCurrentItem(0)
		b.refreshStatus()
		return
	}
	idx := b.listView.GetCurrentItem() + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	b.listView.SetCurrentItem(idx)
	b.refreshStatus()
}

func (b *channelBrowser) movePage(delta int) {
	_, _, _, h := b.listView.GetRect()
	if h <= 1 {
		h = 10
	}
	b.moveSelection(delta * (h - 1))
}

func (b *channelBrowser) handleKey(ev *tcell.EventKey) bool {
	if ev.Key() == tcell.KeyRune && ev.Modifiers() == tcell.ModNone {
		r := ev.Rune()
		if !strings.ContainsRune("\n\r\t", r) && !unicodeIsControl(r) {
			b.appendRune(r)
			return true
		}
	}

	switch ev.Key() {
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		b.backspace()
		return true
	case tcell.KeyUp:
		b.moveSelection(-1)
		return true
	case tcell.KeyDown:
		b.moveSelection(1)
		return true
	case tcell.KeyPgUp:
		b.movePage(-1)
		return true
	case tcell.KeyPgDn:
		b.movePage(1)
		return true
	case tcell.KeyHome:
		b.listView.SetCurrentItem(0)
		b.refreshStatus()
		return true
	case tcell.KeyEnd:
		if len(b.filteredChannels) > 0 {
			b.listView.SetCurrentItem(len(b.filteredChannels) - 1)
		}
		b.refreshStatus()
		return true
	}
	return false
}

func unicodeIsControl(r rune) bool {
	return r < 0x20 || r == 0x7f
}
