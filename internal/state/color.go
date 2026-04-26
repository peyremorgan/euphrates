package state

import (
	"hash/fnv"
	"strings"
)

// channelPalette is the deterministic per-channel color palette, encoded as
// tview color tags. The set was chosen to be readable on both light and dark
// terminal backgrounds.
var channelPalette = []string{
	"[#5fafff]", // sky blue
	"[#5fd75f]", // green
	"[#ffaf00]", // amber
	"[#ff5faf]", // pink
	"[#5fffff]", // cyan
	"[#ffff5f]", // yellow
	"[#af87ff]", // violet
	"[#ff875f]", // salmon
	"[#5fd7af]", // teal
	"[#d75f87]", // magenta
	"[#87afff]", // periwinkle
	"[#afff87]", // chartreuse
	"[#ffaf87]", // peach
	"[#d7afff]", // lavender
	"[#87d7d7]", // mist
	"[#d7d75f]", // olive
}

// dimColor is the tag applied to a target-channel prompt when the channel's
// group is currently hidden.
const dimColor = "[#5f5f5f]"

// resetColor restores tview's default foreground.
const resetColor = "[-]"

// canonicalKey returns the case-folded form used as a channel-map key.
// IRC names are case-insensitive on most networks; lowercasing is a
// reasonable approximation of RFC1459 casemapping for now.
func canonicalKey(name string) string {
	return strings.ToLower(name)
}

// ChannelColor returns the tview color tag deterministically chosen for the
// given channel name. The lookup is case-insensitive.
func ChannelColor(name string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(canonicalKey(name)))
	return channelPalette[int(h.Sum32())%len(channelPalette)]
}

// DimColor returns the color tag used for hidden channels (e.g. on the
// composer prompt when the target channel's group is toggled off).
func DimColor() string { return dimColor }

// ResetColor returns the tview tag that restores default foreground.
func ResetColor() string { return resetColor }
