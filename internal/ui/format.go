// Package ui builds the tview interface and routes input events into state
// mutations and outbound IRC sends.
package ui

import (
	"fmt"
	"strings"

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
	b.WriteString("groups: ")
	for i := 0; i < state.NumGroups; i++ {
		writeMarker(&b, digitForGroup(i), s.IsVisible(state.GroupID(i)))
		if i < state.NumGroups-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteString("  ")
	writeMarker(&b, "S", s.IsVisible(state.GroupServer))
	b.WriteByte(' ')
	writeMarker(&b, "Q", s.IsVisible(state.GroupQueries))

	target := s.Target()
	if target == "" {
		return b.String()
	}
	c, ok := s.Channel(target)
	if !ok {
		return b.String()
	}
	b.WriteString("   ")
	visible := s.IsVisible(c.Group)
	color := state.ChannelColor(c.Name)
	if !visible {
		color = state.DimColor()
	}
	b.WriteString(color)
	b.WriteString("▶ ")
	b.WriteString(state.Escape("[" + channelDisplay(c) + "]"))
	b.WriteString(state.ResetColor())
	return b.String()
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
	text = color + state.Escape(bracketed) + state.ResetColor() + " "
	// Visible width is bracketed runes plus the trailing space.
	width = runeWidth(bracketed) + 1
	return text, width
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

// writeMarker writes a tview-tagged single-character group marker.
// The marker is bold-bright when visible, dim when hidden.
func writeMarker(b *strings.Builder, ch string, visible bool) {
	if visible {
		b.WriteString("[white::b]")
	} else {
		b.WriteString(state.DimColor())
	}
	b.WriteString(ch)
	b.WriteString(state.ResetColor())
}

// runeWidth returns the count of runes in s. Used to size the prompt
// TextView. Note that this approximates display width — full-width or
// combining sequences could under- or over-allocate, but channel names are
// virtually always ASCII so this is fine in practice.
func runeWidth(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// formatEvent is a tiny convenience used by callers that want a compact
// event line. The arrow marker conveys direction without coloring noise.
func formatEvent(arrow, body string) string {
	return fmt.Sprintf("%s %s", arrow, body)
}
