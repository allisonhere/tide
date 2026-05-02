package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"tide/internal/ai"
	"tide/internal/config"
)

func validateAICredentialsCmd(cfg config.AIConfig) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		err := ai.ValidateCredentials(ctx, cfg)
		return AIValidateDoneMsg{Err: err}
	}
}
