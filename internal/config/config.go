package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Theme   string        `toml:"theme"`
	Display DisplayConfig `toml:"display"`
	Feed    FeedConfig    `toml:"feed"`
	Updates UpdatesConfig `toml:"updates"`
	AI      AIConfig      `toml:"ai"`
	Source  SourceConfig  `toml:"source"`
}

type DisplayConfig struct {
	Icons          bool   `toml:"icons"`
	DateFormat     string `toml:"date_format"` // "relative" | "absolute"
	MarkReadOnOpen bool   `toml:"mark_read_on_open"`
	Browser        string `toml:"browser"`
	Density        string `toml:"density"` // "comfortable" | "compact"
}

type FeedConfig struct {
	MaxBodyMiB int `toml:"max_body_mib"`
}

type UpdatesConfig struct {
	CheckOnStartup     bool   `toml:"check_on_startup"`
	CheckIntervalHours int    `toml:"check_interval_hours"`
	LastCheckedUnix    int64  `toml:"last_checked_unix"`
	DismissedVersion   string `toml:"dismissed_version"`
	AvailableVersion   string `toml:"available_version"`
	AvailableSummary   string `toml:"available_summary"`
	AvailablePublished int64  `toml:"available_published_unix"`
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

type SourceConfig struct {
	GReaderURL      string `toml:"greader_url"`
	GReaderLogin    string `toml:"greader_login"`
	GReaderPassword string `toml:"greader_password"`
}

func DefaultConfig() Config {
	return Config{
		Theme: "catppuccin-mocha",
		Display: DisplayConfig{
			Icons:          false,
			DateFormat:     "relative",
			MarkReadOnOpen: true,
			Density:        "compact",
		},
		Feed: FeedConfig{
			MaxBodyMiB: 10,
		},
		Updates: UpdatesConfig{
			CheckOnStartup:     true,
			CheckIntervalHours: 24,
		},
		AI: AIConfig{
			OllamaURL:   "http://localhost:11434",
			OllamaModel: "llama3.2",
			SavePath:    "~/",
		},
		Source: SourceConfig{},
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
	if cfg.Updates.CheckIntervalHours <= 0 {
		cfg.Updates.CheckIntervalHours = DefaultConfig().Updates.CheckIntervalHours
	}
	cfg.Display.Density = NormalizeDisplayDensity(cfg.Display.Density)
	return cfg, nil
}

// NormalizeDisplayDensity returns "comfortable" or "compact".
// Empty or unrecognized values default to "compact".
func NormalizeDisplayDensity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "comfortable":
		return "comfortable"
	default:
		return "compact"
	}
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
