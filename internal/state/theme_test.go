package state

import "testing"

func TestDetectStyleCapability_TrueColor(t *testing.T) {
	env := map[string]string{
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
	}
	cap := DetectStyleCapability(func(k string) string { return env[k] })
	if cap.ColorLevel != ColorLevelTrueColor {
		t.Fatalf("ColorLevel=%v want %v", cap.ColorLevel, ColorLevelTrueColor)
	}
	if !cap.SupportsDim {
		t.Fatal("SupportsDim=false want true")
	}
}

func TestDetectStyleCapability_256AndNoDim(t *testing.T) {
	env := map[string]string{
		"TERM":             "screen-256color",
		"EUPHRATES_NO_DIM": "1",
	}
	cap := DetectStyleCapability(func(k string) string { return env[k] })
	if cap.ColorLevel != ColorLevel256 {
		t.Fatalf("ColorLevel=%v want %v", cap.ColorLevel, ColorLevel256)
	}
	if cap.SupportsDim {
		t.Fatal("SupportsDim=true want false")
	}
}

func TestApplyThemeForCapability_UpdatesPublicColors(t *testing.T) {
	themeMu.Lock()
	oldTheme := activeTheme
	oldPalette := append([]string(nil), channelPalette...)
	oldDim := dimColor
	themeMu.Unlock()
	t.Cleanup(func() {
		themeMu.Lock()
		activeTheme = oldTheme
		channelPalette = append([]string(nil), oldPalette...)
		dimColor = oldDim
		themeMu.Unlock()
	})

	ApplyThemeForCapability(StyleCapability{ColorLevel: ColorLevel16, SupportsDim: false})

	if StatusPrimaryTag() != "[white::b]" {
		t.Fatalf("StatusPrimaryTag()=%q want %q", StatusPrimaryTag(), "[white::b]")
	}
	if DimColor() != "[#6c6f7a]" {
		t.Fatalf("DimColor()=%q want %q", DimColor(), "[#6c6f7a]")
	}
	if ChannelColor("#foo") == "" {
		t.Fatal("ChannelColor returned empty tag")
	}
	if chrome := ActiveChromeTheme(); chrome.StatusBackground == "" || chrome.Separator == "" {
		t.Fatalf("incomplete chrome theme: %#v", chrome)
	}
}
