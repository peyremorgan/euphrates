package state

import (
	"os"
	"strings"
	"sync"
)

// ColorLevel describes the expected terminal color capability.
type ColorLevel int

const (
	// ColorLevel16 targets terminals with basic ANSI color support.
	ColorLevel16 ColorLevel = iota + 1
	// ColorLevel256 targets terminals with extended 256-color support.
	ColorLevel256
	// ColorLevelTrueColor targets terminals that advertise 24-bit color.
	ColorLevelTrueColor
)

// StyleCapability describes terminal capability used for theme fallback.
type StyleCapability struct {
	ColorLevel  ColorLevel
	SupportsDim bool
}

// ChromeTheme contains colors used by the static UI chrome.
type ChromeTheme struct {
	StatusForeground string
	StatusBackground string
	Separator        string
	EventsForeground string
	PromptForeground string
}

type themeProfile struct {
	statusPrimaryTag string
	channelPalette   []string
	dimColor         string
	chrome           ChromeTheme
}

var themeMu sync.RWMutex

var activeTheme = themeForCapability(DefaultStyleCapability())

// StatusPrimaryTag returns the bright style tag used by status markers.
func StatusPrimaryTag() string {
	themeMu.RLock()
	defer themeMu.RUnlock()
	return activeTheme.statusPrimaryTag
}

// ActiveChromeTheme returns a snapshot of static UI chrome colors.
func ActiveChromeTheme() ChromeTheme {
	themeMu.RLock()
	defer themeMu.RUnlock()
	return activeTheme.chrome
}

// ApplyDefaultThemeFromEnv configures the active theme based on environment.
func ApplyDefaultThemeFromEnv() {
	ApplyThemeForCapability(DefaultStyleCapability())
}

// ApplyThemeForCapability configures the active theme for the provided
// terminal capability profile.
func ApplyThemeForCapability(cap StyleCapability) {
	themeMu.Lock()
	defer themeMu.Unlock()
	activeTheme = themeForCapability(cap)
	channelPalette = append([]string(nil), activeTheme.channelPalette...)
	dimColor = activeTheme.dimColor
}

// DefaultStyleCapability detects capability from standard terminal variables.
func DefaultStyleCapability() StyleCapability {
	return DetectStyleCapability(os.Getenv)
}

// DetectStyleCapability infers color depth and dim support from env vars.
func DetectStyleCapability(getenv func(string) string) StyleCapability {
	term := strings.ToLower(getenv("TERM"))
	colorTerm := strings.ToLower(getenv("COLORTERM"))

	level := ColorLevel16
	if strings.Contains(colorTerm, "truecolor") || strings.Contains(colorTerm, "24bit") {
		level = ColorLevelTrueColor
	} else if strings.Contains(term, "256color") {
		level = ColorLevel256
	}

	dim := term != "dumb" && getenv("EUPHRATES_NO_DIM") != "1"
	return StyleCapability{ColorLevel: level, SupportsDim: dim}
}

func themeForCapability(cap StyleCapability) themeProfile {
	theme := themeNeonNoir16()
	switch cap.ColorLevel {
	case ColorLevelTrueColor:
		theme = themeNeonNoirTrueColor()
	case ColorLevel256:
		theme = themeNeonNoir256()
	}
	if !cap.SupportsDim {
		theme.dimColor = "[#6c6f7a]"
	}
	return theme
}

func themeNeonNoirTrueColor() themeProfile {
	return themeProfile{
		statusPrimaryTag: "[#d8f7ff::b]",
		channelPalette: []string{
			"[#57d7ff]", "[#39d7c2]", "[#75e885]", "[#8dc7ff]",
			"[#d09dff]", "[#f58fca]", "[#ff9f78]", "[#ffc86e]",
			"[#7de7ff]", "[#5ec9ff]", "[#69d9b8]", "[#a2f0a0]",
			"[#9cc0ff]", "[#d8adff]", "[#f3a6d4]", "[#ffc2a2]",
		},
		dimColor: "[#5a5f6f]",
		chrome: ChromeTheme{
			StatusForeground: "#d8f7ff",
			StatusBackground: "#10131a",
			Separator:        "#2c8ca4",
			EventsForeground: "#9ec8d3",
			PromptForeground: "#b6cad1",
		},
	}
}

func themeNeonNoir256() themeProfile {
	return themeProfile{
		statusPrimaryTag: "[#d7ffff::b]",
		channelPalette: []string{
			"[#5fd7ff]", "[#5fd7af]", "[#87d787]", "[#87afff]",
			"[#af87ff]", "[#d787d7]", "[#ffaf87]", "[#ffd787]",
			"[#87ffff]", "[#5fafff]", "[#5fd7d7]", "[#afff87]",
			"[#afafff]", "[#d7afff]", "[#ffafd7]", "[#ffd7af]",
		},
		dimColor: "[#5f5f6f]",
		chrome: ChromeTheme{
			StatusForeground: "lightcyan",
			StatusBackground: "#1c1c24",
			Separator:        "#5f87af",
			EventsForeground: "#87afaf",
			PromptForeground: "#afc6d1",
		},
	}
}

func themeNeonNoir16() themeProfile {
	return themeProfile{
		statusPrimaryTag: "[white::b]",
		channelPalette: []string{
			"[teal]", "[aqua]", "[green]", "[lightblue]",
			"[mediumpurple]", "[fuchsia]", "[lightsalmon]", "[yellow]",
			"[cyan]", "[deepskyblue]", "[aquamarine]", "[lightgreen]",
			"[skyblue]", "[plum]", "[pink]", "[wheat]",
		},
		dimColor: "[gray]",
		chrome: ChromeTheme{
			StatusForeground: "white",
			StatusBackground: "black",
			Separator:        "gray",
			EventsForeground: "silver",
			PromptForeground: "silver",
		},
	}
}
