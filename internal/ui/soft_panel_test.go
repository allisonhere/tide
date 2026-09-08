package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestSoftPanelBoxEmbedsTitleInTopBorder(t *testing.T) {
	chrome := newManagerChrome(40, CatppuccinMocha, false)
	box := renderSoftPanelBox("hello", 40, "tide", "settings", chrome)

	lines := strings.Split(box, "\n")
	top := ansi.Strip(lines[0])
	if !strings.HasPrefix(top, "╭─ tide · settings ") {
		t.Fatalf("expected embedded title in top border, got %q", top)
	}
	if !strings.HasSuffix(top, "╮") {
		t.Fatalf("expected top border to end with ╮, got %q", top)
	}
	if got := lipgloss.Width(lines[0]); got != 42 {
		t.Fatalf("expected top border to span width+2 = 42 cells, got %d", got)
	}
	bottom := ansi.Strip(lines[len(lines)-1])
	if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
		t.Fatalf("expected rounded bottom corners, got %q", bottom)
	}
}

// The plain vt52 theme must embed the title in the ASCII top rule the same way
// the rounded themes do — not render it as a separate row below the border.
func TestSoftPanelBoxPlainUIEmbedsTitleInASCIITopBorder(t *testing.T) {
	chrome := newManagerChrome(40, VT52, true)
	box := renderSoftPanelBox("hello", 40, "tide", "settings", chrome)

	if strings.ContainsAny(box, "╭╮╰╯─│") {
		t.Fatalf("expected plainUI soft panel to avoid unicode box drawing, got %q", box)
	}

	lines := strings.Split(box, "\n")
	top := ansi.Strip(lines[0])
	if !strings.HasPrefix(top, "+- tide · settings ") {
		t.Fatalf("expected embedded title in ASCII top border, got %q", top)
	}
	if !strings.HasSuffix(top, "+") {
		t.Fatalf("expected ASCII top border to end with +, got %q", top)
	}
	if got := lipgloss.Width(lines[0]); got != 42 {
		t.Fatalf("expected top border to span width+2 = 42 cells, got %d", got)
	}
	if row1 := ansi.Strip(lines[1]); strings.Contains(row1, "tide · settings") {
		t.Fatalf("title should not repeat on the first body row, got %q", row1)
	}
	bottom := ansi.Strip(lines[len(lines)-1])
	if !strings.HasPrefix(bottom, "+") || !strings.HasSuffix(bottom, "+") {
		t.Fatalf("expected ASCII bottom corners, got %q", bottom)
	}
}
