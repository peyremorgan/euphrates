// Package ui builds the tview interface and routes input events into state
// mutations and outbound IRC sends.
package ui

import (
	"strconv"
	"strings"

	"github.com/rivo/tview"

	"euphrates/internal/state"
)

// digitForGroup returns the keyboard digit ('1'..'9' then '0') that toggles
// the numeric group with the given zero-based index.
func digitForGroup(i int) string {
	if i == state.NumGroups-1 {
		return "0"
	}
	return string(rune('1' + i))
}

// FormatStatus renders the status line shown above the main pane: a digit per
// numeric group (bright if visible, dim if hidden), the special "S"/"Q"
// markers for the server and queries groups, and the current send target.
//
// The output is a single line of tview-tagged text.
func FormatStatus(s *state.State) string {
	var b strings.Builder
	serverName := s.ServerName()
	if serverName == "" {
		serverName = "server"
	}
	b.WriteString("[white::b]")
	b.WriteString(state.Escape(serverName))
	b.WriteString(state.ResetColor())
	b.WriteString("  ")
	for i := 0; i < state.NumGroups; i++ {
		gid := state.GroupID(i)
		writeMarker(&b, digitForGroup(i)+brailleForCount(s.NumericGroupCount(gid)), s.IsVisible(gid))
		if i < state.NumGroups-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteString("  ")
	writeMarker(&b, "S", s.IsVisible(state.GroupServer))
	b.WriteByte(' ')
	writeMarker(&b, "Q", s.IsVisible(state.GroupQueries))
	return b.String()
}

// FormatStatusCount renders the normal-channel total shown on the right side
// of the status row.
func FormatStatusCount(s *state.State) string {
	count := s.NormalChannelCount()
	label := "Channels"
	if count == 1 {
		label = "Channel"
	}
	return state.Escape(strconv.Itoa(count) + " " + label)
}

// FormatPrompt renders the composer prompt for the current target, plus the
// printable width (in cells) of the *visible* portion. The returned text
// includes a trailing space for separation from the input field. When there
// is no target, returns ("", 0).
func FormatPrompt(s *state.State) (text string, width int) {
	target := s.Target()
	if target == "" {
		return "", 0
	}
	c, ok := s.Channel(target)
	if !ok {
		return "", 0
	}
	visible := s.IsVisible(c.Group)
	display := channelDisplay(c) // "#foo" / "@alice" / "*server"
	bracketed := "[" + display + "]"
	color := state.ChannelColor(c.Name)
	if !visible {
		color = state.DimColor()
	}
	escaped := state.Escape(bracketed)
	text = color + escaped + state.ResetColor() + " "
	// Visible width includes literal brackets plus trailing space.
	width = tview.TaggedStringWidth(escaped) + 1
	return text, width
}

// brailleForCount returns a compact 8-dot braille indicator for n channels.
// Counts above 8 are saturated to a full cell.
func brailleForCount(n int) string {
	const full = "⣿"
	if n > 8 {
		return full
	}
	if n < 0 {
		n = 0
	}
	glyphs := [9]string{"⠀", "⡀", "⡄", "⡆", "⡇", "⣇", "⣧", "⣷", full}
	return glyphs[n]
}

// channelDisplay returns the string shown in the bracketed prefix:
// channel name as-is, queries get "@" sigil, server stays as "*server".
func channelDisplay(c state.Channel) string {
	switch c.Kind {
	case state.ChanQuery:
		return "@" + strings.TrimPrefix(c.Name, "@")
	default:
		return c.Name
	}
}

// writeMarker writes a tview-tagged group token.
// The token is bold-bright when visible, dim when hidden.
func writeMarker(b *strings.Builder, ch string, visible bool) {
	if visible {
		b.WriteString("[white::b]")
	} else {
		b.WriteString(state.DimColor())
	}
	b.WriteString(ch)
	b.WriteString(state.ResetColor())
}
