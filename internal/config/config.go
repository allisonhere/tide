package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Theme   string        `toml:"theme"`
	Display DisplayConfig `toml:"display"`
	Feed    FeedConfig    `toml:"feed"`
	AI      AIConfig      `toml:"ai"`
}

type DisplayConfig struct {
	Icons          bool   `toml:"icons"`
	DateFormat     string `toml:"date_format"` // "relative" | "absolute"
	MarkReadOnOpen bool   `toml:"mark_read_on_open"`
	Browser        string `toml:"browser"`
}

type FeedConfig struct {
	MaxBodyMiB int `toml:"max_body_mib"`
}

type AIConfig struct {
	Provider    string `toml:"provider"` // "openai" | "claude" | "gemini" | "ollama" | ""
	OpenAIKey   string `toml:"openai_key"`
	ClaudeKey   string `toml:"claude_key"`
	GeminiKey   string `toml:"gemini_key"`
	OllamaURL   string `toml:"ollama_url"`
	OllamaModel string `toml:"ollama_model"`
	SavePath    string `toml:"save_path"`
}

func DefaultConfig() Config {
	return Config{
		Theme: "catppuccin-mocha",
		Display: DisplayConfig{
			Icons:          false,
			DateFormat:     "relative",
			MarkReadOnOpen: true,
		},
		Feed: FeedConfig{
			MaxBodyMiB: 10,
		},
		AI: AIConfig{
			OllamaURL:   "http://localhost:11434",
			OllamaModel: "llama3.2",
			SavePath:    "~/",
		},
	}
}

func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}

	// Start from defaults so missing keys keep their default values.
	cfg := DefaultConfig()
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return DefaultConfig(), err
	}
	if cfg.Feed.MaxBodyMiB <= 0 {
		cfg.Feed.MaxBodyMiB = DefaultConfig().Feed.MaxBodyMiB
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "rss", "config.toml"), nil
}
