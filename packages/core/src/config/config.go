package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Home         string
	BaseURL      string
	APIKey       string
	Model        string
	Agent        string
	SystemPrompt string
	ProjectRoot  string
}

func Load(projectRoot string) (Config, error) {
	home, err := dataHome()
	if err != nil {
		return Config{}, err
	}
	return Config{
		Home:         home,
		BaseURL:      first(os.Getenv("YTEAM_BASE_URL"), "https://opencode.ai/zen/v1"),
		APIKey:       os.Getenv("YTEAM_API_KEY"),
		Model:        first(os.Getenv("YTEAM_MODEL"), "mimo-v2.5-free"),
		Agent:        first(os.Getenv("YTEAM_AGENT"), "build"),
		SystemPrompt: os.Getenv("YTEAM_SYSTEM_PROMPT"),
		ProjectRoot:  projectRoot,
	}, nil
}

func dataHome() (string, error) {
	if value := os.Getenv("YTEAM_HOME"); value != "" {
		return filepath.Abs(value)
	}
	if value := os.Getenv("APPDATA"); value != "" {
		return filepath.Join(value, "yteam"), nil
	}
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return filepath.Join(value, "yteam"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "yteam"), nil
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
