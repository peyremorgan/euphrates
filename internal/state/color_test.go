package state

import "testing"

func TestChannelColor_Deterministic(t *testing.T) {
	a := ChannelColor("#archiveteam-bs")
	b := ChannelColor("#archiveteam-bs")
	if a != b {
		t.Errorf("not deterministic: %q vs %q", a, b)
	}
}

func TestChannelColor_CaseInsensitive(t *testing.T) {
	if ChannelColor("#Foo") != ChannelColor("#foo") {
		t.Errorf("case sensitivity leaked into hash")
	}
}

func TestChannelColor_FromPalette(t *testing.T) {
	got := ChannelColor("#anything")
	found := false
	for _, c := range channelPalette {
		if c == got {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("color %q not in palette", got)
	}
}

func TestChannelColor_DistributesAcrossPalette(t *testing.T) {
	// Sanity check: many distinct names should hit at least half the palette.
	seen := map[string]bool{}
	names := []string{
		"#a", "#b", "#c", "#d", "#e", "#foo", "#bar", "#baz",
		"#archiveteam", "#go-nuts", "#libera", "#linux", "#rust",
		"#python", "#kde", "#gnome", "#vim", "#emacs", "#nix", "#ops",
	}
	for _, n := range names {
		seen[ChannelColor(n)] = true
	}
	if len(seen) < len(channelPalette)/2 {
		t.Errorf("poor distribution: %d distinct colors", len(seen))
	}
}

func TestCanonicalKey(t *testing.T) {
	if canonicalKey("#FOO") != "#foo" {
		t.Errorf("canonicalKey didn't lower-case")
	}
}
