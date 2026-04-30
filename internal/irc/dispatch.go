// Package irc wraps ergochat/irc-go's ircevent connection and translates
// inbound IRC messages into state mutations exposed by the UI.
//
// The package is split into two layers: a pure dispatcher (this file) that
// turns parsed IRC commands into Handlers callbacks, and a thin Connection
// wrapper (client.go) that owns the ircevent.Connection and wires its
// callbacks into the dispatcher. The split keeps protocol routing easy to
// unit-test without a network.
package irc

import (
	"fmt"
	"strings"
	"time"

	"github.com/ergochat/irc-go/ircmsg"

	"euphrates/internal/state"
)

// Handlers is the set of callbacks the dispatcher invokes. Every field is
// optional; nil callbacks are silently skipped so tests and partial
// integrations can opt in to subsets.
type Handlers struct {
	// Self returns our current nickname. Used to distinguish self-events
	// (which mutate state) from others' events (which become log entries).
	Self func() string

	// OnMessage is fired for PRIVMSG/NOTICE/CTCP_ACTION addressed to a
	// channel or to us directly.
	OnMessage func(state.Message)

	// OnEvent is fired for non-message activity: joins/parts of others,
	// topic changes, mode changes, kicks, quits, errors, server numerics.
	OnEvent func(string)

	// OnJoin is fired when *we* join a channel, so the UI can register it.
	OnJoin func(channel string)

	// OnPart is fired when *we* leave a channel.
	OnPart func(channel string)

	// OnChannelList is fired when a LIST response has fully completed.
	OnChannelList func(names []string)
}

func (h Handlers) self() string {
	if h.Self == nil {
		return ""
	}
	return h.Self()
}

func (h Handlers) emitMessage(m state.Message) {
	if h.OnMessage != nil {
		h.OnMessage(m)
	}
}

func (h Handlers) emitEvent(line string) {
	if h.OnEvent != nil {
		h.OnEvent(line)
	}
}

func (h Handlers) emitEventNow(line string) {
	h.emitEvent(formatStampedLine(time.Now(), line))
}

func formatStampedLine(ts time.Time, line string) string {
	return ts.Format("15:04:05") + " " + line
}

func (h Handlers) emitJoin(channel string) {
	if h.OnJoin != nil {
		h.OnJoin(channel)
	}
}

func (h Handlers) emitPart(channel string) {
	if h.OnPart != nil {
		h.OnPart(channel)
	}
}

func (h Handlers) emitChannelList(names []string) {
	if h.OnChannelList != nil {
		h.OnChannelList(names)
	}
}

// dispatchPrivmsg routes a PRIVMSG (or rewritten CTCP_ACTION) into a
// state.Message. For messages addressed to us directly (target == self), the
// message is associated with a query channel keyed by the sender's nick.
func dispatchPrivmsg(h Handlers, src, target, text string, kind state.MessageKind) {
	nick := nickFromSource(src)
	channel := target
	if strings.EqualFold(target, h.self()) {
		// Direct message: store under the sender's query channel.
		channel = nick
	}
	if channel == "" || nick == "" {
		return
	}
	h.emitMessage(state.Message{
		Channel: channel,
		Nick:    nick,
		Text:    text,
		Kind:    kind,
		Time:    time.Now(),
	})
}

// dispatchJoin handles a JOIN. If the joining nick is us, register the
// channel; otherwise log a synthetic event.
func dispatchJoin(h Handlers, src, channel string) {
	if channel == "" {
		return
	}
	nick := nickFromSource(src)
	if strings.EqualFold(nick, h.self()) {
		h.emitJoin(channel)
		h.emitEventNow(fmt.Sprintf("→ joined %s", channel))
		return
	}
	h.emitEventNow(fmt.Sprintf("→ %s joined %s", nick, channel))
}

// dispatchPart handles a PART. When we leave, drop the channel from state;
// otherwise log a synthetic event.
func dispatchPart(h Handlers, src, channel, reason string) {
	if channel == "" {
		return
	}
	nick := nickFromSource(src)
	suffix := ""
	if reason != "" {
		suffix = " (" + reason + ")"
	}
	if strings.EqualFold(nick, h.self()) {
		h.emitPart(channel)
		h.emitEventNow(fmt.Sprintf("← left %s%s", channel, suffix))
		return
	}
	h.emitEventNow(fmt.Sprintf("← %s left %s%s", nick, channel, suffix))
}

// dispatchQuit handles a QUIT. We can't know which channels were affected
// without tracking membership; the event line is sufficient.
func dispatchQuit(h Handlers, src, reason string) {
	nick := nickFromSource(src)
	if nick == "" {
		return
	}
	suffix := ""
	if reason != "" {
		suffix = " (" + reason + ")"
	}
	h.emitEventNow(fmt.Sprintf("← %s quit%s", nick, suffix))
}

// dispatchNick handles a NICK change. Self changes are rare (servers
// sometimes rename us); we still surface them as an event for visibility.
func dispatchNick(h Handlers, src, newNick string) {
	old := nickFromSource(src)
	if old == "" || newNick == "" {
		return
	}
	if strings.EqualFold(old, h.self()) || strings.EqualFold(newNick, h.self()) {
		h.emitEventNow(fmt.Sprintf("∗ you are now %s", newNick))
		return
	}
	h.emitEventNow(fmt.Sprintf("∗ %s is now %s", old, newNick))
}

// dispatchTopic handles a TOPIC command sent live during a session.
func dispatchTopic(h Handlers, src, channel, topic string) {
	if channel == "" {
		return
	}
	nick := nickFromSource(src)
	h.emitEventNow(fmt.Sprintf("# %s topic by %s: %s", channel, nick, topic))
}

// dispatchKick handles a KICK. Self-kicks remove the channel.
func dispatchKick(h Handlers, src, channel, target, reason string) {
	if channel == "" || target == "" {
		return
	}
	by := nickFromSource(src)
	suffix := ""
	if reason != "" {
		suffix = " (" + reason + ")"
	}
	if strings.EqualFold(target, h.self()) {
		h.emitPart(channel)
		h.emitEventNow(fmt.Sprintf("⨯ kicked from %s by %s%s", channel, by, suffix))
		return
	}
	h.emitEventNow(fmt.Sprintf("⨯ %s kicked %s from %s%s", by, target, channel, suffix))
}

// dispatchMode renders a MODE change as a single event line.
func dispatchMode(h Handlers, src, target string, params []string) {
	if target == "" {
		return
	}
	by := nickFromSource(src)
	if by == "" {
		by = "server"
	}
	h.emitEventNow(fmt.Sprintf("± %s mode %s by %s", target, strings.Join(params, " "), by))
}

// dispatchServerNumeric routes a server numeric reply (001, 372, 376, 433, …)
// to a server-channel message so the user can scroll back through MOTDs and
// notices.
func dispatchServerNumeric(h Handlers, command string, params []string) {
	text := serverNumericText(params)
	if text == "" {
		return
	}
	h.emitMessage(state.Message{
		Channel: state.ServerChannelName,
		Nick:    command,
		Text:    text,
		Kind:    state.KindServer,
		Time:    time.Now(),
	})
}

// dispatchNotice routes a NOTICE. If addressed to a channel, it lands as a
// channel message; if addressed to us or to "*" (pre-registration), it goes
// to the server log.
func dispatchNotice(h Handlers, src, target, text string) {
	if target == "" || target == "*" || strings.EqualFold(target, h.self()) {
		h.emitMessage(state.Message{
			Channel: state.ServerChannelName,
			Nick:    nickOrServer(src),
			Text:    text,
			Kind:    state.KindNotice,
			Time:    time.Now(),
		})
		return
	}
	dispatchPrivmsg(h, src, target, text, state.KindNotice)
}

// nickFromSource extracts the nick from a `nick!user@host` source. If the
// source has no `!`, we treat it as either a server name (returned empty
// to suppress) or a bare nick.
func nickFromSource(src string) string {
	if i := strings.IndexByte(src, '!'); i >= 0 {
		return src[:i]
	}
	if strings.ContainsAny(src, ".") {
		return ""
	}
	return src
}

// nickOrServer returns the nick portion if present, else the literal source
// (used for NOTICEs that come from server names).
func nickOrServer(src string) string {
	if n := nickFromSource(src); n != "" {
		return n
	}
	if src == "" {
		return "*"
	}
	return src
}

// serverNumericText pulls the human-readable text out of a numeric reply.
// IRC numerics conventionally place the message in the trailing parameter;
// we drop the first parameter which is our own nick.
func serverNumericText(params []string) string {
	switch len(params) {
	case 0:
		return ""
	case 1:
		return params[0]
	default:
		// Drop the leading nick target; join the remainder with spaces.
		return strings.Join(params[1:], " ")
	}
}

// param fetches params[i] or "" when out of range. Used by the connection
// adapter to defensively unpack ircmsg.Message slices.
func param(m ircmsg.Message, i int) string {
	if i < 0 || i >= len(m.Params) {
		return ""
	}
	return m.Params[i]
}
