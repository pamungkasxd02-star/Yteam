package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Home         string `json:"home,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	Model        string `json:"model,omitempty"`
	Agent        string `json:"agent,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	ProjectRoot  string `json:"project_root,omitempty"`
}

type fileConfig struct {
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	Model        string `json:"model,omitempty"`
	Agent        string `json:"agent,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

func Load(projectRoot string) (Config, error) {
	home, err := dataHome()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Home:        home,
		BaseURL:     "https://opencode.ai/zen/v1",
		Model:       "mimo-v2.5-free",
		Agent:       "build",
		ProjectRoot: projectRoot,
	}
	for _, path := range configPaths(home, projectRoot) {
		values, exists, err := readFile(path)
		if err != nil {
			return Config{}, err
		}
		if exists {
			applyFileConfig(&cfg, values)
		}
	}
	if value := strings.TrimSpace(os.Getenv("YTEAM_BASE_URL")); value != "" {
		cfg.BaseURL = value
	}
	if value, ok := os.LookupEnv("YTEAM_API_KEY"); ok {
		cfg.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("YTEAM_MODEL")); value != "" {
		cfg.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("YTEAM_AGENT")); value != "" {
		cfg.Agent = value
	}
	if value := os.Getenv("YTEAM_SYSTEM_PROMPT"); value != "" {
		cfg.SystemPrompt = value
	}
	return cfg, nil
}

func configPaths(home, projectRoot string) []string {
	paths := []string{}
	explicit := strings.TrimSpace(os.Getenv("YTEAM_CONFIG"))
	if explicit != "" {
		paths = append(paths, explicit)
	}
	// Explicit config is intentionally applied after the normal files below.
	normal := []string{filepath.Join(home, "config.json")}
	if strings.TrimSpace(projectRoot) != "" {
		normal = append(normal, filepath.Join(projectRoot, "yteam.json"), filepath.Join(projectRoot, ".yteam.json"), filepath.Join(projectRoot, ".yteam", "config.json"))
	}
	if explicit == "" {
		return normal
	}
	return append(normal, paths...)
}

func readFile(path string) (fileConfig, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileConfig{}, false, nil
	}
	if err != nil {
		return fileConfig{}, false, err
	}
	var values fileConfig
	if err := json.Unmarshal(data, &values); err != nil {
		return fileConfig{}, false, errors.New("invalid config " + path + ": " + err.Error())
	}
	return values, true, nil
}

func applyFileConfig(cfg *Config, values fileConfig) {
	if values.BaseURL != "" {
		cfg.BaseURL = values.BaseURL
	}
	if values.APIKey != "" {
		cfg.APIKey = values.APIKey
	}
	if values.Model != "" {
		cfg.Model = values.Model
	}
	if values.Agent != "" {
		cfg.Agent = values.Agent
	}
	if values.SystemPrompt != "" {
		cfg.SystemPrompt = values.SystemPrompt
	}
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
