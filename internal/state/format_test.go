package state

import (
	"strings"
	"testing"
	"time"
)

func fixedMessageTime() time.Time {
	return time.Date(2026, time.April, 28, 8, 4, 39, 0, time.UTC)
}

func TestEscape_NeutralisesTags(t *testing.T) {
	in := "look [red]not red[-]"
	out := Escape(in)
	if !strings.Contains(out, "[red[]") {
		t.Errorf("did not escape: %q", out)
	}
}

func TestEscape_NoBrackets(t *testing.T) {
	if Escape("hello world") != "hello world" {
		t.Error("non-bracket content modified")
	}
}

func TestEscape_PermissiveBeyondTviewStock(t *testing.T) {
	// `@` is not in tview's stock Escape character class but our Escape
	// neutralises it too, so the prompt label stays a single literal.
	if Escape("[@alice]") != "[@alice[]" {
		t.Errorf("@-prefixed escape: %q", Escape("[@alice]"))
	}
}

func TestChannelTag_EscapesBrackets(t *testing.T) {
	tag := channelTag("#foo", ChanNormal)
	// must contain the literal channel name with the trailing escape "[]".
	if !strings.Contains(tag, "[#foo[]") {
		t.Errorf("channelTag output missing escaped prefix: %q", tag)
	}
	// must end with reset.
	if !strings.HasSuffix(tag, resetColor) {
		t.Errorf("channelTag missing reset: %q", tag)
	}
}

func TestChannelTag_QueryPrefix(t *testing.T) {
	tag := channelTag("alice", ChanQuery)
	if !strings.Contains(tag, "[@alice[]") {
		t.Errorf("query tag form: %q", tag)
	}
}

func TestChannelTag_ServerPrefix(t *testing.T) {
	tag := channelTag(ServerChannelName, ChanServer)
	if !strings.Contains(tag, "[*server[]") {
		t.Errorf("server tag form: %q", tag)
	}
}

func TestFormatMessage_Privmsg(t *testing.T) {
	m := Message{Channel: "#go", Nick: "ada", Text: "hi", Kind: KindPrivmsg, Time: fixedMessageTime()}
	got := formatMessage(m, ChanNormal)
	wantNick := UserColor("ada") + "ada" + resetColor
	if !strings.HasPrefix(got, "08:04:39 ") {
		t.Errorf("missing timestamp prefix: %q", got)
	}
	if !strings.Contains(got, "<"+wantNick+">") {
		t.Errorf("missing <nick>: %q", got)
	}
	if !strings.HasSuffix(got, "hi") {
		t.Errorf("missing trailing body: %q", got)
	}
}

func TestFormatMessage_Action(t *testing.T) {
	m := Message{Channel: "#go", Nick: "ada", Text: "waves", Kind: KindAction, Time: fixedMessageTime()}
	got := formatMessage(m, ChanNormal)
	wantNick := UserColor("ada") + "ada" + resetColor
	if !strings.Contains(got, " * "+wantNick+" waves") {
		t.Errorf("action body wrong: %q", got)
	}
}

func TestFormatMessage_Notice(t *testing.T) {
	m := Message{Channel: "#go", Nick: "srv", Text: "hello", Kind: KindNotice, Time: fixedMessageTime()}
	got := formatMessage(m, ChanNormal)
	wantNick := UserColor("srv") + "srv" + resetColor
	if !strings.Contains(got, " -"+wantNick+"- hello") {
		t.Errorf("notice body wrong: %q", got)
	}
}

func TestFormatMessage_Server(t *testing.T) {
	m := Message{Channel: ServerChannelName, Text: "MOTD line", Kind: KindServer, Time: fixedMessageTime()}
	got := formatMessage(m, ChanServer)
	if !strings.Contains(got, "[*server[]") {
		t.Errorf("server tag missing: %q", got)
	}
	if !strings.HasSuffix(got, "MOTD line") {
		t.Errorf("server body missing: %q", got)
	}
}

func TestFormatMessage_EscapesUserContent(t *testing.T) {
	m := Message{Channel: "#x", Nick: "bob", Text: "look [red]hi[-]", Kind: KindPrivmsg, Time: fixedMessageTime()}
	got := formatMessage(m, ChanNormal)
	if !strings.Contains(got, "[red[]") {
		t.Errorf("user text not escaped: %q", got)
	}
	if !strings.Contains(got, UserColor("bob")+"bob"+resetColor) {
		t.Errorf("nick not colorized: %q", got)
	}
}

func TestFormatMessage_EscapesNickBeforeColor(t *testing.T) {
	m := Message{Channel: "#x", Nick: "[@alice]", Text: "hi", Kind: KindPrivmsg, Time: fixedMessageTime()}
	got := formatMessage(m, ChanNormal)
	wantNick := UserColor("[@alice]") + "[@alice[]" + resetColor
	if !strings.Contains(got, "<"+wantNick+">") {
		t.Errorf("nick not escaped+colorized correctly: %q", got)
	}
}
