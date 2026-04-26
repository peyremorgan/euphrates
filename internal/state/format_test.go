package state

import (
	"strings"
	"testing"
)

func TestEscapeContent_NeutralisesTags(t *testing.T) {
	in := "look [red]not red[-]"
	out := escapeContent(in)
	if !strings.Contains(out, "[red[]") {
		t.Errorf("did not escape: %q", out)
	}
	if strings.Contains(out, "[red]") {
		// the original tag must no longer appear unescaped
		// (it appears as "[red[]")
		// regex: ensure "[red]" is not present as a standalone tag
		// by checking it's not followed-by something other than '['
		idx := strings.Index(out, "[red]")
		if idx >= 0 && (idx+5 >= len(out) || out[idx+5] != '[') {
			t.Errorf("unescaped tag survived: %q", out)
		}
	}
}

func TestEscapeContent_NoBrackets(t *testing.T) {
	if escapeContent("hello world") != "hello world" {
		t.Error("non-bracket content modified")
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
	m := Message{Channel: "#go", Nick: "ada", Text: "hi", Kind: KindPrivmsg}
	got := formatMessage(m, ChanNormal)
	if !strings.Contains(got, "<ada>") {
		t.Errorf("missing <nick>: %q", got)
	}
	if !strings.HasSuffix(got, "hi") {
		t.Errorf("missing trailing body: %q", got)
	}
}

func TestFormatMessage_Action(t *testing.T) {
	m := Message{Channel: "#go", Nick: "ada", Text: "waves", Kind: KindAction}
	got := formatMessage(m, ChanNormal)
	if !strings.Contains(got, " * ada waves") {
		t.Errorf("action body wrong: %q", got)
	}
}

func TestFormatMessage_Notice(t *testing.T) {
	m := Message{Channel: "#go", Nick: "srv", Text: "hello", Kind: KindNotice}
	got := formatMessage(m, ChanNormal)
	if !strings.Contains(got, " -srv- hello") {
		t.Errorf("notice body wrong: %q", got)
	}
}

func TestFormatMessage_Server(t *testing.T) {
	m := Message{Channel: ServerChannelName, Text: "MOTD line", Kind: KindServer}
	got := formatMessage(m, ChanServer)
	if !strings.Contains(got, "[*server[]") {
		t.Errorf("server tag missing: %q", got)
	}
	if !strings.HasSuffix(got, "MOTD line") {
		t.Errorf("server body missing: %q", got)
	}
}

func TestFormatMessage_EscapesUserContent(t *testing.T) {
	m := Message{Channel: "#x", Nick: "bob", Text: "look [red]hi[-]", Kind: KindPrivmsg}
	got := formatMessage(m, ChanNormal)
	if !strings.Contains(got, "[red[]") {
		t.Errorf("user text not escaped: %q", got)
	}
}
