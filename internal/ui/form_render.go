package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func formLabelWidth(width int) int {
	if width < 34 {
		return max(10, width/2)
	}
	return min(28, max(22, width/2+4))
}

func renderFormGroupTitle(label string, width int, chrome managerChrome) string {
	marker := "━"
	rule := "─"
	if chrome.plainUI {
		marker = "="
		rule = "-"
	}
	titleText := strings.ToUpper(strings.TrimSpace(label))
	title := lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.text).
		Bold(true).
		Render(marker + " " + truncate(titleText, max(1, width-2)))
	row := title
	ruleW := max(0, width-lipgloss.Width(row)-1)
	if ruleW > 0 {
		row += lipgloss.NewStyle().Background(chrome.baseBg).Render(" ")
		row += lipgloss.NewStyle().
			Background(chrome.baseBg).
			Foreground(chrome.muted).
			Render(strings.Repeat(rule, ruleW))
	}
	if gap := width - lipgloss.Width(row); gap > 0 {
		row += lipgloss.NewStyle().Background(chrome.baseBg).Render(strings.Repeat(" ", gap))
	}
	return row
}

func renderFormRow(label string, focused bool, control string, width, labelW int, chrome managerChrome) string {
	rowBg := chrome.baseBg
	labelFg := chrome.muted
	markerFg := chrome.muted
	if focused {
		rowBg = chrome.surfaceBg
		labelFg = chrome.highlightFg
		markerFg = chrome.accent
	}

	marker := " "
	if focused {
		if chrome.plainUI {
			marker = ">"
		} else {
			marker = "▌"
		}
	}
	markerCell := lipgloss.NewStyle().
		Background(rowBg).
		Foreground(markerFg).
		Width(2).
		Render(marker)
	labelW = min(labelW, max(1, width-lipgloss.Width(markerCell)-1))
	controlW := max(1, width-lipgloss.Width(markerCell)-labelW)
	labelCell := lipgloss.NewStyle().
		Background(rowBg).
		Foreground(labelFg).
		Bold(focused).
		Width(labelW).
		Render(truncate(label, max(1, labelW-1)))
	control = truncateStyled(control, controlW, chrome.baseBg)
	controlCell := lipgloss.NewStyle().Background(rowBg).Width(controlW).Render(control)
	return markerCell + labelCell + controlCell
}

func renderFormFieldHeader(label string, focused bool, status string, width int, chrome managerChrome) string {
	rowBg := chrome.baseBg
	labelFg := chrome.muted
	markerFg := chrome.muted
	if focused {
		rowBg = chrome.surfaceBg
		labelFg = chrome.highlightFg
		markerFg = chrome.accent
	}

	marker := " "
	if focused {
		if chrome.plainUI {
			marker = ">"
		} else {
			marker = "▌"
		}
	}
	markerCell := lipgloss.NewStyle().
		Background(rowBg).
		Foreground(markerFg).
		Width(2).
		Render(marker)
	labelW := max(1, width-lipgloss.Width(markerCell))
	row := markerCell + lipgloss.NewStyle().
		Background(rowBg).
		Foreground(labelFg).
		Bold(focused).
		Render(truncate(label, labelW))
	if status != "" {
		badge := renderFormBadge(status, focused, chrome)
		gap := max(1, width-lipgloss.Width(row)-lipgloss.Width(badge))
		row += lipgloss.NewStyle().Background(rowBg).Render(strings.Repeat(" ", gap))
		row += badge
	}
	if gap := width - lipgloss.Width(row); gap > 0 {
		row += lipgloss.NewStyle().Background(rowBg).Render(strings.Repeat(" ", gap))
	}
	return row
}

func renderFormHint(text string, width int, chrome managerChrome) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	}
	prefix := "  "
	if !chrome.plainUI {
		prefix = "  · "
	}
	textW := max(1, width-lipgloss.Width(prefix))
	return lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.muted).
		Width(width).
		Render(prefix + truncate(text, textW))
}

func renderFormInlineStatus(text string, width int, chrome managerChrome) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return lipgloss.NewStyle().Background(chrome.baseBg).Width(width).Render("")
	}
	return lipgloss.NewStyle().
		Background(chrome.baseBg).
		Foreground(chrome.muted).
		Width(width).
		Render(truncate(text, width))
}

func renderFormBadge(text string, focused bool, chrome managerChrome) string {
	text = strings.ToUpper(strings.TrimSpace(text))
	badgeW := max(7, lipgloss.Width(text)+2)
	if focused {
		return chrome.key.UnsetPadding().Width(badgeW).Align(lipgloss.Center).Render(text)
	}
	return lipgloss.NewStyle().
		Background(chrome.surfaceBg).
		Foreground(chrome.muted).
		Width(badgeW).
		Align(lipgloss.Center).
		Render(strings.ToLower(text))
}

func truncateStyled(s string, width int, bg lipgloss.Color) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	plain := ansi.Strip(s)
	if lipgloss.Width(plain) <= width {
		return s
	}
	return lipgloss.NewStyle().Background(bg).Render(truncate(plain, width))
}
