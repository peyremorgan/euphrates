package ui

import (
	"strings"
	"testing"

	"euphrates/internal/state"
)

func newSt() *state.State {
	return state.New(state.Config{MessageCap: 100, EventCap: 5, ServerName: "irc.example.org"})
}

func TestDigitForGroup(t *testing.T) {
	want := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0"}
	for i, w := range want {
		if got := digitForGroup(i); got != w {
			t.Errorf("digitForGroup(%d)=%q want %q", i, got, w)
		}
	}
}

func TestFormatStatus_DefaultEverythingVisible(t *testing.T) {
	s := newSt()
	got := FormatStatus(s)
	if !strings.HasPrefix(got, state.StatusPrimaryTag()+"irc.example.org[-]  ") {
		t.Errorf("status missing server prefix: %q", got)
	}
	// All ten digits + S + Q each carry the bright marker.
	if strings.Contains(got, state.DimColor()) {
		t.Errorf("default status has dim marker: %q", got)
	}
	for _, d := range []string{"1", "9", "0", "S", "Q"} {
		if !strings.Contains(got, d) {
			t.Errorf("missing %q marker: %q", d, got)
		}
	}
}

func TestFormatStatus_UsesServerFallbackWhenUnset(t *testing.T) {
	s := state.New(state.Config{MessageCap: 100, EventCap: 5})
	got := FormatStatus(s)
	if !strings.HasPrefix(got, state.StatusPrimaryTag()+"server[-]  ") {
		t.Errorf("status missing fallback server prefix: %q", got)
	}
}

func TestFormatStatus_ShowsBrailleGroupCounts(t *testing.T) {
	s := newSt()
	s.EnsureChannel("#chan")
	got := FormatStatus(s)
	if !strings.Contains(got, "1⡀") {
		t.Errorf("group 1 count not rendered: %q", got)
	}
	if !strings.Contains(got, "2⠀") {
		t.Errorf("group 2 zero-count marker not rendered: %q", got)
	}
}

func TestFormatStatus_DimsHidden(t *testing.T) {
	s := newSt()
	s.SetVisible(state.GroupID(2), false)
	s.SetVisible(state.GroupServer, false)
	got := FormatStatus(s)
	if !strings.Contains(got, state.DimColor()) {
		t.Errorf("expected dim markers: %q", got)
	}
}

func TestFormatStatus_NoTarget(t *testing.T) {
	s := newSt()
	got := FormatStatus(s)
	if strings.Contains(got, "▶") {
		t.Errorf("unexpected target arrow: %q", got)
	}
}

func TestFormatStatusCount_UsesSingularAndPlural(t *testing.T) {
	s := newSt()
	if got := FormatStatusCount(s); got != "0 Channels" {
		t.Errorf("count=%q want %q", got, "0 Channels")
	}

	s.EnsureChannel("#foo")
	if got := FormatStatusCount(s); got != "1 Channel" {
		t.Errorf("count=%q want %q", got, "1 Channel")
	}

	s.EnsureChannel("#bar")
	if got := FormatStatusCount(s); got != "2 Channels" {
		t.Errorf("count=%q want %q", got, "2 Channels")
	}
}

func TestFormatStatusCount_ExcludesQueriesAndServer(t *testing.T) {
	s := newSt()
	s.EnsureChannel("alice")
	s.EnsureChannel(state.ServerChannelName)
	s.EnsureChannel("#foo")

	if got := FormatStatusCount(s); got != "1 Channel" {
		t.Errorf("count=%q want %q", got, "1 Channel")
	}
}

func TestFormatPrompt_Visible(t *testing.T) {
	s := newSt()
	s.EnsureChannel("#foo")
	text, width := FormatPrompt(s)
	if !strings.Contains(text, "[#foo[]") {
		t.Errorf("prompt missing channel: %q", text)
	}
	if !strings.HasSuffix(text, " ") {
		t.Errorf("prompt missing trailing space: %q", text)
	}
	if width != 7 {
		t.Errorf("width=%d want 7", width)
	}
}

func TestFormatPrompt_HiddenIsDim(t *testing.T) {
	s := newSt()
	s.EnsureChannel("#foo")
	c, _ := s.Channel("#foo")
	s.SetVisible(c.Group, false)
	text, _ := FormatPrompt(s)
	if !strings.HasPrefix(text, state.DimColor()) {
		t.Errorf("hidden prompt not dim: %q", text)
	}
}

func TestFormatPrompt_QueryUsesAtSigil(t *testing.T) {
	s := newSt()
	s.EnsureChannel("alice")
	text, _ := FormatPrompt(s)
	if !strings.Contains(text, "[@alice[]") {
		t.Errorf("query prompt: %q", text)
	}
}

func TestFormatPrompt_NoTarget(t *testing.T) {
	s := newSt()
	text, width := FormatPrompt(s)
	if text != "" || width != 0 {
		t.Errorf("no-target prompt: %q %d", text, width)
	}
}

func TestBrailleForCount_ProgressiveAndSaturated(t *testing.T) {
	cases := []struct {
		count int
		want  string
	}{
		{0, "⠀"},
		{4, "⡇"},
		{8, "⣿"},
		{99, "⣿"},
	}
	for _, c := range cases {
		if got := brailleForCount(c.count); got != c.want {
			t.Errorf("brailleForCount(%d)=%q want %q", c.count, got, c.want)
		}
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{name: "empty", in: nil, want: ""},
		{name: "single", in: []string{"#go"}, want: "#go"},
		{name: "case insensitive", in: []string{"#GoLang", "#gol"}, want: "#GoL"},
		{name: "none", in: []string{"#go", "#rust"}, want: "#"},
		{name: "partial", in: []string{"#go-help", "#go-nuts"}, want: "#go-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := longestCommonPrefix(tc.in); got != tc.want {
				t.Fatalf("lcp=%q want %q", got, tc.want)
			}
		})
	}
}

func TestFormatCompletionLines(t *testing.T) {
	t.Run("single line", func(t *testing.T) {
		got := formatCompletionLines([]string{"#a", "#b", "#c"}, 20)
		if len(got) != 1 {
			t.Fatalf("lines=%v", got)
		}
		if got[0] != "#a #b #c" {
			t.Fatalf("line=%q", got[0])
		}
	})

	t.Run("two lines with overflow summary", func(t *testing.T) {
		got := formatCompletionLines(
			[]string{"#alpha", "#beta", "#gamma", "#delta", "#epsilon", "#zeta"},
			18,
		)
		if len(got) > 2 {
			t.Fatalf("too many lines: %v", got)
		}
		if len(got) == 0 {
			t.Fatal("no output lines")
		}
		if !strings.Contains(got[len(got)-1], "+") {
			t.Fatalf("expected overflow summary, got %v", got)
		}
	})
}
