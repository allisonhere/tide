package image

import (
	"strconv"
	"strings"
)

// Protocol names the terminal graphics protocol a Renderer speaks. Only "kitty"
// is implemented today; the field exists so SIXEL / iTerm can be added later
// without changing call sites.
type Protocol string

const (
	ProtocolNone  Protocol = ""
	ProtocolKitty Protocol = "kitty"
)

// Capability is the result of terminal graphics detection.
type Capability struct {
	Supported bool
	Protocol  Protocol
	// CellW / CellH are the pixel dimensions of one terminal cell, used for
	// image sizing. Zero means unknown (callers fall back to 8x16).
	CellW int
	CellH int
	// Reason is a short human-readable explanation, surfaced in logs / hints.
	Reason string
}

// fallback cell size when the terminal does not report pixel geometry.
const (
	fallbackCellW = 8
	fallbackCellH = 16
)

// CellSize returns CellW/CellH, substituting the 8x16 fallback for unknown (0)
// values.
func (c Capability) CellSize() (int, int) {
	w, h := c.CellW, c.CellH
	if w <= 0 {
		w = fallbackCellW
	}
	if h <= 0 {
		h = fallbackCellH
	}
	return w, h
}

// Detect determines whether the current terminal can render Kitty graphics.
//
// getenv is injected (pass os.Getenv) so the environment matrix is testable.
// When activeProbe is true and the environment is inconclusive, Detect performs
// a short, bounded round-trip query against the controlling terminal (unix
// only, and only safe to call before a raw-mode TUI takes over stdin). Env-only
// detection is instant and always safe.
func Detect(getenv func(string) string, activeProbe bool) Capability {
	cap, envConclusive := detectFromEnv(getenv)

	if !envConclusive && activeProbe {
		if probed, ok := activeDetect(); ok {
			cap = probed
		}
	}

	// Nail down the cell pixel size whenever images will actually be drawn.
	// detectFromEnv/activeDetect may already have it; otherwise ask the
	// terminal (safe only pre-TUI, hence gated on activeProbe).
	if cap.Supported && (cap.CellW == 0 || cap.CellH == 0) {
		if w, h := queryCellSize(activeProbe); w > 0 && h > 0 {
			cap.CellW, cap.CellH = w, h
		}
	}

	if !cap.Supported && cap.Reason == "" {
		cap.Reason = "no known Kitty-graphics terminal detected"
	}
	return cap
}

// detectFromEnv recognises terminals that advertise Kitty-graphics support via
// environment variables. The bool is false when the environment says nothing
// conclusive.
func detectFromEnv(getenv func(string) string) (Capability, bool) {
	env := func(k string) string { return strings.TrimSpace(getenv(k)) }

	term := strings.ToLower(env("TERM"))
	termProgram := strings.ToLower(env("TERM_PROGRAM"))

	switch {
	case term == "xterm-kitty" || env("KITTY_WINDOW_ID") != "":
		return Capability{Supported: true, Protocol: ProtocolKitty, Reason: "kitty"}, true

	case term == "xterm-ghostty" || env("GHOSTTY_RESOURCES_DIR") != "" || termProgram == "ghostty":
		return Capability{Supported: true, Protocol: ProtocolKitty, Reason: "ghostty"}, true

	case env("WEZTERM_EXECUTABLE") != "" || termProgram == "wezterm":
		return Capability{Supported: true, Protocol: ProtocolKitty, Reason: "wezterm"}, true

	case env("KONSOLE_VERSION") != "":
		if v, err := strconv.Atoi(env("KONSOLE_VERSION")); err == nil && v >= 220400 {
			return Capability{Supported: true, Protocol: ProtocolKitty, Reason: "konsole"}, true
		}
		return Capability{Reason: "konsole too old for kitty graphics"}, true
	}

	return Capability{}, false
}
