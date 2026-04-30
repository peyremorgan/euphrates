package irc

import (
	"bufio"
	"fmt"
	"math/rand/v2"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"euphrates/internal/state"
)

type testRecorder struct {
	mu     sync.Mutex
	msgs   []state.Message
	events []string
	joins  []string
	parts  []string
	msgCh  chan state.Message
	joinCh chan string
}

func newTestRecorder() *testRecorder {
	return &testRecorder{
		msgCh:  make(chan state.Message, 64),
		joinCh: make(chan string, 16),
	}
}

func (r *testRecorder) handlers() Handlers {
	return Handlers{
		OnMessage: func(m state.Message) {
			r.mu.Lock()
			r.msgs = append(r.msgs, m)
			r.mu.Unlock()
			select {
			case r.msgCh <- m:
			default:
			}
		},
		OnEvent: func(line string) {
			r.mu.Lock()
			r.events = append(r.events, line)
			r.mu.Unlock()
		},
		OnJoin: func(ch string) {
			r.mu.Lock()
			r.joins = append(r.joins, ch)
			r.mu.Unlock()
			select {
			case r.joinCh <- ch:
			default:
			}
		},
		OnPart: func(ch string) {
			r.mu.Lock()
			r.parts = append(r.parts, ch)
			r.mu.Unlock()
		},
	}
}

func waitForJoin(t *testing.T, rec *testRecorder, channel string) {
	t.Helper()
	deadline := time.After(8 * time.Second)
	for {
		select {
		case got := <-rec.joinCh:
			if strings.EqualFold(got, channel) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for join %q", channel)
		}
	}
}

func waitForMessage(t *testing.T, rec *testRecorder, pred func(state.Message) bool) state.Message {
	t.Helper()
	deadline := time.After(8 * time.Second)
	for {
		select {
		case got := <-rec.msgCh:
			if pred(got) {
				return got
			}
		case <-deadline:
			t.Fatal("timed out waiting for matching message")
		}
	}
}

func randomNick(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, rand.IntN(1_000_000))
}

func runTwoClientScenario(t *testing.T, serverAddr string) {
	t.Helper()
	channel := fmt.Sprintf("#e2e-%d", rand.IntN(1_000_000))
	nickA := randomNick("alice")
	nickB := randomNick("bob")

	recA := newTestRecorder()
	recB := newTestRecorder()

	clientA, err := New(Config{Server: serverAddr, Nick: nickA, Channels: []string{channel}}, recA.handlers())
	if err != nil {
		t.Fatalf("new client A: %v", err)
	}
	clientB, err := New(Config{Server: serverAddr, Nick: nickB, Channels: []string{channel}}, recB.handlers())
	if err != nil {
		t.Fatalf("new client B: %v", err)
	}

	if err := clientA.Connect(); err != nil {
		t.Fatalf("connect A: %v", err)
	}
	if err := clientB.Connect(); err != nil {
		t.Fatalf("connect B: %v", err)
	}

	loopDone := make(chan struct{}, 2)
	go func() {
		clientA.Loop()
		loopDone <- struct{}{}
	}()
	go func() {
		clientB.Loop()
		loopDone <- struct{}{}
	}()

	defer func() {
		clientA.Quit("bye")
		clientB.Quit("bye")
		select {
		case <-loopDone:
		case <-time.After(2 * time.Second):
		}
		select {
		case <-loopDone:
		case <-time.After(2 * time.Second):
		}
	}()

	waitForJoin(t, recA, channel)
	waitForJoin(t, recB, channel)

	if err := clientA.SendPrivmsg(channel, "hello from A"); err != nil {
		t.Fatalf("send privmsg: %v", err)
	}
	msg := waitForMessage(t, recB, func(m state.Message) bool {
		return strings.EqualFold(m.Channel, channel) && m.Text == "hello from A" && strings.EqualFold(m.Nick, clientA.Nick()) && m.Kind == state.KindPrivmsg
	})
	if msg.Time.IsZero() {
		t.Fatal("expected non-zero timestamp on channel message")
	}

	if err := clientA.SendAction(channel, "waves"); err != nil {
		t.Fatalf("send action: %v", err)
	}
	_ = waitForMessage(t, recB, func(m state.Message) bool {
		return strings.EqualFold(m.Channel, channel) && m.Text == "waves" && strings.EqualFold(m.Nick, clientA.Nick()) && m.Kind == state.KindAction
	})

	if err := clientA.SendPrivmsg(clientB.Nick(), "direct hi"); err != nil {
		t.Fatalf("send direct message: %v", err)
	}
	dm := waitForMessage(t, recB, func(m state.Message) bool {
		return strings.EqualFold(m.Channel, clientA.Nick()) && m.Text == "direct hi" && m.Kind == state.KindPrivmsg
	})
	if !strings.EqualFold(dm.Nick, clientA.Nick()) {
		t.Fatalf("direct message nick=%q want %q", dm.Nick, clientA.Nick())
	}
}

type miniIRCServer struct {
	ln      net.Listener
	server  string
	mu      sync.Mutex
	clients map[*miniClient]struct{}
}

type miniClient struct {
	srv        *miniIRCServer
	conn       net.Conn
	nick       string
	hasNick    bool
	hasUser    bool
	registered bool
	channels   map[string]struct{}
}

func startMiniIRCServer(t *testing.T) (*miniIRCServer, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &miniIRCServer{
		ln:      ln,
		server:  "mini.test",
		clients: make(map[*miniClient]struct{}),
	}
	go s.acceptLoop()
	return s, ln.Addr().String()
}

func (s *miniIRCServer) close() {
	_ = s.ln.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		_ = c.conn.Close()
	}
}

func (s *miniIRCServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		c := &miniClient{srv: s, conn: conn, channels: make(map[string]struct{})}
		s.mu.Lock()
		s.clients[c] = struct{}{}
		s.mu.Unlock()
		go c.loop()
	}
}

func (s *miniIRCServer) remove(c *miniClient) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
}

func (s *miniIRCServer) withClients(fn func(*miniClient)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		fn(c)
	}
}

func (c *miniClient) loop() {
	defer func() {
		c.srv.remove(c)
		_ = c.conn.Close()
	}()

	r := bufio.NewReader(c.conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		c.handleLine(strings.TrimRight(line, "\r\n"))
	}
}

func (c *miniClient) handleLine(line string) {
	if line == "" {
		return
	}
	cmd, params := parseIRCLine(line)
	switch strings.ToUpper(cmd) {
	case "CAP":
		if len(params) >= 2 && strings.EqualFold(params[1], "LS") {
			c.writef(":%s CAP * LS :\r\n", c.srv.server)
		}
	case "NICK":
		if len(params) >= 1 {
			c.nick = params[0]
			c.hasNick = true
			c.maybeRegister()
		}
	case "USER":
		c.hasUser = true
		c.maybeRegister()
	case "PING":
		if len(params) > 0 {
			c.writef("PONG :%s\r\n", params[len(params)-1])
		}
	case "JOIN":
		if len(params) >= 1 {
			for _, ch := range strings.Split(params[0], ",") {
				ch = strings.TrimSpace(ch)
				if ch == "" {
					continue
				}
				c.channels[strings.ToLower(ch)] = struct{}{}
				c.broadcastChannel(ch, ":%s!u@localhost JOIN %s", c.nick, ch)
			}
		}
	case "PRIVMSG":
		if len(params) < 2 {
			return
		}
		target := params[0]
		text := params[1]
		if strings.HasPrefix(target, "#") {
			c.broadcastChannel(target, ":%s!u@localhost PRIVMSG %s :%s", c.nick, target, text)
			return
		}
		c.srv.withClients(func(other *miniClient) {
			if strings.EqualFold(other.nick, target) {
				other.writef(":%s!u@localhost PRIVMSG %s :%s\r\n", c.nick, target, text)
			}
		})
	case "QUIT":
		reason := "Client Quit"
		if len(params) > 0 {
			reason = params[len(params)-1]
		}
		for ch := range c.channels {
			c.broadcastChannel(ch, ":%s!u@localhost QUIT :%s", c.nick, reason)
		}
		_ = c.conn.Close()
	}
}

func (c *miniClient) maybeRegister() {
	if c.registered || !c.hasNick || !c.hasUser {
		return
	}
	c.registered = true
	c.writef(":%s 001 %s :Welcome\r\n", c.srv.server, c.nick)
	c.writef(":%s 376 %s :End of MOTD\r\n", c.srv.server, c.nick)
}

func (c *miniClient) broadcastChannel(channel, format string, args ...any) {
	low := strings.ToLower(channel)
	payload := fmt.Sprintf(format, args...)
	c.srv.withClients(func(other *miniClient) {
		if _, ok := other.channels[low]; ok {
			other.writef("%s\r\n", payload)
		}
	})
}

func (c *miniClient) writef(format string, args ...any) {
	_, _ = fmt.Fprintf(c.conn, format, args...)
}

func parseIRCLine(line string) (string, []string) {
	if line == "" {
		return "", nil
	}
	if line[0] == ':' {
		if i := strings.IndexByte(line, ' '); i >= 0 {
			line = strings.TrimSpace(line[i+1:])
		} else {
			return "", nil
		}
	}
	var args []string
	for len(line) > 0 {
		if line[0] == ':' {
			args = append(args, line[1:])
			break
		}
		i := strings.IndexByte(line, ' ')
		if i < 0 {
			args = append(args, line)
			break
		}
		if i > 0 {
			args = append(args, line[:i])
		}
		line = strings.TrimLeft(line[i+1:], " ")
	}
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}
