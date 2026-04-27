package state

import (
	"hash/fnv"
	"strings"
)

// channelPalette is the deterministic per-channel color palette, encoded as
// tview color tags. It is seeded from the active theme profile.
var channelPalette []string

// dimColor is the tag applied to a target-channel prompt when the channel's
// group is currently hidden. It is seeded from the active theme profile.
var dimColor string

// resetColor restores tview's default foreground.
const resetColor = "[-]"

func init() {
	ApplyDefaultThemeFromEnv()
}

// canonicalKey returns the case-folded form used as a channel-map key.
// IRC names are case-insensitive on most networks; lowercasing is a
// reasonable approximation of RFC1459 casemapping for now.
func canonicalKey(name string) string {
	return strings.ToLower(name)
}

// deterministicColorTag returns a stable palette color tag for a name.
// It is shared by both channel and nick color helpers.
func deterministicColorTag(name string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(canonicalKey(name)))
	return channelPalette[int(h.Sum32())%len(channelPalette)]
}

// ChannelColor returns the tview color tag deterministically chosen for the
// given channel name. The lookup is case-insensitive.
func ChannelColor(name string) string {
	return deterministicColorTag(name)
}

// UserColor returns the tview color tag deterministically chosen for the
// given nick. The lookup is case-insensitive.
func UserColor(nick string) string {
	return deterministicColorTag(nick)
}

// DimColor returns the color tag used for hidden channels (e.g. on the
// composer prompt when the target channel's group is toggled off).
func DimColor() string { return dimColor }

// ResetColor returns the tview tag that restores default foreground.
func ResetColor() string { return resetColor }
