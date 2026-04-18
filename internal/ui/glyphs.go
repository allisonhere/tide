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
