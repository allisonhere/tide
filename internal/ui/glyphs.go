package ui

// Presentation helpers for the vt52 theme (ASCII) vs default Unicode styling.

func (s Styles) StatusBarSepText() string {
	if s.PlainUI {
		return " | "
	}
	return "  ·  "
}

func (s Styles) ThemePickerCursor() string {
	if s.PlainUI {
		return "> "
	}
	return "▶ "
}

func (s Styles) InlineMidDot() string {
	if s.PlainUI {
		return " | "
	}
	return " · "
}

// StarGlyph marks a saved article. The ASCII fallback covers both the vt52
// theme and terminals configured without icons.
func (s Styles) StarGlyph() string {
	if s.PlainUI {
		return "*"
	}
	return "★"
}

func aiConnectionStatusGlyph(plain bool, state aiConnectionState) string {
	if plain {
		switch state {
		case aiConnectionPending:
			return "..."
		case aiConnectionSuccess:
			return "OK"
		case aiConnectionError:
			return "ERR"
		default:
			return "o"
		}
	}
	switch state {
	case aiConnectionPending:
		return "◔"
	case aiConnectionSuccess:
		return "●"
	case aiConnectionError:
		return "●"
	default:
		return "○"
	}
}
