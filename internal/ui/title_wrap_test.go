package ui

import (
	"strings"
	"testing"

	"github.com/allisonhere/tide/internal/config"
	"github.com/allisonhere/tide/internal/db"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestWrappedTitlesAndScrolling(t *testing.T) {
	for _, density := range []string{"compact", "comfortable"} {
		t.Run(density, func(t *testing.T) {
			m := Model{cfg: config.DefaultConfig(), width: 90, height: 36, styles: BuildStyles(GruvboxLight, density, "square")}
			m.cfg.Display.WrapListTitles = true
			for i := 0; i < 12; i++ {
				m.filteredArticles = append(m.filteredArticles, db.Article{Title: strings.Repeat("Headline word ", 12), Starred: i%2 == 0})
			}
			for i := range m.filteredArticles {
				m.articleCursor = i
				m.ensureArticleCursorVisible()
				if i < m.listOffset || i >= m.listOffset+m.articleRowsVisible() {
					t.Fatalf("cursor %d outside window from %d", i, m.listOffset)
				}
				used := 0
				for j := m.listOffset; j < m.listOffset+m.articleRowsVisible(); j++ {
					used += lipgloss.Height(m.renderWrappedArticleRow(j))
				}
				if used > m.articlesPaneContentHeight()-m.paneHeaderHeight() {
					t.Fatalf("rows overflow: %d", used)
				}
			}
		})
	}
}

func TestArticleTitleWrapPreservesText(t *testing.T) {
	m := Model{width: 70, height: 30, styles: BuildStyles(GruvboxLight, "comfortable", "square")}
	m.cfg.Display.WrapArticleTitle = true
	title := "A headline with enough words to span multiple lines and retain its final distinctive ending"
	got := ansi.Strip(m.renderArticleContent(db.Article{Title: title, Content: "body"}))
	if !strings.Contains(got, "distinctive ending") || strings.Contains(got, "…") {
		t.Fatalf("title clipped: %s", got)
	}
}
