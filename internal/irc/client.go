package irc

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/ergochat/irc-go/ircevent"
	"github.com/ergochat/irc-go/ircmsg"

	"euphrates/internal/state"
)

// Config configures the IRC connection. Sensible zero values: UseTLS=false,
// Port=6667 (or 6697 if UseTLS), and Channels empty (autojoin nothing).
type Config struct {
	Server   string // host[:port]
	UseTLS   bool   // wrap connection in TLS
	Nick     string
	User     string // ident; defaults to Nick when empty
	RealName string // gecos; defaults to Nick when empty
	Password string // server (PASS) password
	SASLUser string // SASL PLAIN credentials (optional)
	SASLPass string
	Channels []string // autojoin on connect
}

// Client owns an ircevent.Connection and an associated Handlers struct.
// Methods Nick, SendPrivmsg, SendAction, Join, Part, Quit make Client an
// implementation of ui.Sender.
type Client struct {
	conn     *ircevent.Connection
	handlers Handlers
	cfg      Config
	listBuf  []string
}

// New constructs a Client from the given config and handlers. The Handlers'
// Self field is overridden to point at the live ircevent connection so it
// always reflects the server-assigned nickname.
func New(cfg Config, h Handlers) (*Client, error) {
	if cfg.Server == "" {
		return nil, errors.New("irc: Server is required")
	}
	if cfg.Nick == "" {
		return nil, errors.New("irc: Nick is required")
	}
	user := cfg.User
	if user == "" {
		user = cfg.Nick
	}
	real := cfg.RealName
	if real == "" {
		real = cfg.Nick
	}

	conn := &ircevent.Connection{
		Server:     cfg.Server,
		Nick:       cfg.Nick,
		User:       user,
		RealName:   real,
		Password:   cfg.Password,
		UseTLS:     cfg.UseTLS,
		EnableCTCP: true,
	}
	if cfg.UseTLS {
		conn.TLSConfig = &tls.Config{ServerName: HostOnly(cfg.Server)}
	}
	if cfg.SASLUser != "" {
		conn.UseSASL = true
		conn.SASLLogin = cfg.SASLUser
		conn.SASLPassword = cfg.SASLPass
	}

	c := &Client{conn: conn, cfg: cfg}
	c.handlers = h
	c.handlers.Self = func() string { return conn.CurrentNick() }
	c.registerCallbacks()
	return c, nil
}

// Connect opens the connection. Loop must be called afterwards on the
// goroutine that should drive callbacks.
func (c *Client) Connect() error { return c.conn.Connect() }

// SetHandlers replaces the inbound-event handlers. Self is always overridden
// to track the live ircevent connection, so callers can leave it nil. Useful
// when the UI needs the Client to construct its Sender before its own
// callback methods are bound.
//
// Must be called before Connect; not safe to call concurrently with active
// callbacks.
func (c *Client) SetHandlers(h Handlers) {
	c.handlers = h
	c.handlers.Self = func() string { return c.conn.CurrentNick() }
}

// Loop blocks until the connection ends. Call from a dedicated goroutine.
func (c *Client) Loop() { c.conn.Loop() }

// Nick returns the server-assigned current nickname (may differ from the
// configured Nick if the server collided/renamed us).
func (c *Client) Nick() string { return c.conn.CurrentNick() }

// SendPrivmsg sends a regular PRIVMSG.
func (c *Client) SendPrivmsg(target, text string) error {
	return c.conn.Privmsg(target, text)
}

// SendAction sends a CTCP ACTION ("/me ...").
func (c *Client) SendAction(target, text string) error {
	return c.conn.Action(target, text)
}

// Join asks the server to join channel.
func (c *Client) Join(channel string) error {
	return c.conn.Join(channel)
}

// Part asks the server to leave channel. If reason is non-empty, it is sent
// as the PART message suffix.
func (c *Client) Part(channel, reason string) error {
	if reason == "" {
		return c.conn.Part(channel)
	}
	return c.conn.Send("PART", channel, reason)
}

// Quit closes the connection with the given reason.
func (c *Client) Quit(reason string) {
	c.conn.QuitMessage = reason
	c.conn.Quit()
}

// registerCallbacks wires ircevent commands to the dispatcher.
func (c *Client) registerCallbacks() {
	c.conn.AddConnectCallback(func(_ ircmsg.Message) {
		for _, ch := range c.cfg.Channels {
			if ch == "" {
				continue
			}
			_ = c.conn.Join(ch)
		}
		c.listBuf = nil
		_ = c.conn.Send("LIST")
	})

	c.conn.AddCallback("PRIVMSG", func(m ircmsg.Message) {
		dispatchPrivmsg(c.handlers, m.Source, param(m, 0), param(m, 1), state.KindPrivmsg)
	})
	c.conn.AddCallback("CTCP_ACTION", func(m ircmsg.Message) {
		dispatchPrivmsg(c.handlers, m.Source, param(m, 0), param(m, 1), state.KindAction)
	})
	c.conn.AddCallback("NOTICE", func(m ircmsg.Message) {
		dispatchNotice(c.handlers, m.Source, param(m, 0), param(m, 1))
	})
	c.conn.AddCallback("JOIN", func(m ircmsg.Message) {
		dispatchJoin(c.handlers, m.Source, param(m, 0))
	})
	c.conn.AddCallback("PART", func(m ircmsg.Message) {
		dispatchPart(c.handlers, m.Source, param(m, 0), param(m, 1))
	})
	c.conn.AddCallback("QUIT", func(m ircmsg.Message) {
		dispatchQuit(c.handlers, m.Source, param(m, 0))
	})
	c.conn.AddCallback("NICK", func(m ircmsg.Message) {
		dispatchNick(c.handlers, m.Source, param(m, 0))
	})
	c.conn.AddCallback("TOPIC", func(m ircmsg.Message) {
		dispatchTopic(c.handlers, m.Source, param(m, 0), param(m, 1))
	})
	c.conn.AddCallback("KICK", func(m ircmsg.Message) {
		dispatchKick(c.handlers, m.Source, param(m, 0), param(m, 1), param(m, 2))
	})
	c.conn.AddCallback("MODE", func(m ircmsg.Message) {
		dispatchMode(c.handlers, m.Source, param(m, 0), m.Params[1:])
	})
	c.conn.AddCallback("ERROR", func(m ircmsg.Message) {
		c.handlers.emitEventNow("⨯ server error: " + strings.Join(m.Params, " "))
	})
	c.conn.AddCallback("322", func(m ircmsg.Message) {
		channel := param(m, 1)
		if channel == "" {
			return
		}
		c.listBuf = append(c.listBuf, channel)
	})
	c.conn.AddCallback("323", func(_ ircmsg.Message) {
		c.handlers.emitChannelList(c.listBuf)
		c.listBuf = nil
	})
	c.conn.AddCallback("353", func(m ircmsg.Message) {
		dispatchNamesReply(c.handlers, m.Params)
	})

	// Server numerics 001..599. Skip a couple that ircevent already drives
	// or that are pure protocol noise.
	for code := 1; code <= 599; code++ {
		if code == 322 || code == 323 {
			continue
		}
		cmd := fmt.Sprintf("%03d", code)
		c.conn.AddCallback(cmd, func(m ircmsg.Message) {
			dispatchServerNumeric(c.handlers, m.Command, m.Params)
		})
	}
}
