package state

import (
	"regexp"
	"strings"
)

// MessageKind classifies how a Message is rendered.
type MessageKind int

const (
	// KindPrivmsg is a regular channel or query PRIVMSG.
	KindPrivmsg MessageKind = iota
	// KindAction is a CTCP ACTION ("/me ...").
	KindAction
	// KindNotice is a NOTICE.
	KindNotice
	// KindServer is a server-originated line (numerics, MOTD, errors).
	// Nick is unused; Channel is the synthetic ServerChannelName.
	KindServer
)

// ServerChannelName is the synthetic channel name used to bucket server
// messages (numerics, MOTD, errors) into the server group.
const ServerChannelName = "*server"

// Message is the immutable record of a chat or server-originated line.
type Message struct {
	Channel string
	Nick    string
	Text    string
	Kind    MessageKind
}

// tviewEscapePattern mirrors tview's Escape regex but with a permissive
// character class so any [...] pair (including ones tview's stock parser
// would ignore, like "[@alice]") is neutralised. Defined locally to avoid
// importing tview into the state package.
var tviewEscapePattern = regexp.MustCompile(`\[([^[\]]*)\]`)

// Escape neutralises tview color tags inside arbitrary content (nicks,
// message text, channel-prefix labels). Any bracketed substring `[xxx]`
// becomes `[xxx[]`, which tview renders as the literal `[xxx]`.
//
// This is the in-tree equivalent of tview.Escape, with a wider pattern so
// non-color-shaped strings like `[@alice]` are also neutralised.
func Escape(s string) string {
	return tviewEscapePattern.ReplaceAllString(s, "[$1[]")
}

// channelTag returns a colored, tview-safe rendering of the bracketed
// channel prefix used as a message prefix in the main pane:
//
//	[#archiveteam-bs]   (channel)
//	[@alice]            (query, indicated by an @ sigil)
//	[*server]           (server messages)
//
// The `[` and `]` of the visible prefix are escaped so tview won't try to
// parse them as a color tag.
func channelTag(name string, kind ChanKind) string {
	var prefix string
	switch kind {
	case ChanQuery:
		prefix = "@" + strings.TrimPrefix(name, "@")
	case ChanServer:
		prefix = name // already starts with '*'
	default:
		prefix = name
	}
	color := ChannelColor(name)
	// Build "[fg]" + literal "[prefix]" (escaped) + "[-]"
	return color + "[" + prefix + "[]" + resetColor
}

// formatMessage renders a Message into a single tview-safe line (no
// trailing newline) for the main scroll pane.
func formatMessage(m Message, kind ChanKind) string {
	tag := channelTag(m.Channel, kind)
	body := Escape(m.Text)
	switch m.Kind {
	case KindAction:
		return tag + " * " + Escape(m.Nick) + " " + body
	case KindNotice:
		return tag + " -" + Escape(m.Nick) + "- " + body
	case KindServer:
		return tag + " " + body
	default: // KindPrivmsg
		return tag + " <" + Escape(m.Nick) + "> " + body
	}
}
