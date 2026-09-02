package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPortableDefaults(t *testing.T) {
	t.Setenv("YTEAM_HOME", t.TempDir())
	t.Setenv("YTEAM_BASE_URL", "")
	t.Setenv("YTEAM_MODEL", "")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("base URL = %q", cfg.BaseURL)
	}
	if cfg.Model != "mimo-v2.5-free" {
		t.Fatalf("model = %q", cfg.Model)
	}
	if cfg.Agent != "build" {
		t.Fatalf("agent = %q", cfg.Agent)
	}
}

func TestLoadConfigPrecedenceIsDeterministic(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	write := func(path string, value fileConfig) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, "config.json"), fileConfig{BaseURL: "user", Model: "user-model", Agent: "plan", SystemPrompt: "user-prompt"})
	write(filepath.Join(root, "yteam.json"), fileConfig{BaseURL: "project", Model: "project-model", SystemPrompt: "project-prompt"})
	explicit := filepath.Join(t.TempDir(), "explicit.json")
	write(explicit, fileConfig{BaseURL: "explicit", Model: "explicit-model"})
	t.Setenv("YTEAM_HOME", home)
	t.Setenv("YTEAM_CONFIG", explicit)
	t.Setenv("YTEAM_BASE_URL", "environment")
	t.Setenv("YTEAM_MODEL", "environment-model")
	t.Setenv("YTEAM_AGENT", "build")
	t.Setenv("YTEAM_SYSTEM_PROMPT", "environment-prompt")
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "environment" || cfg.Model != "environment-model" || cfg.Agent != "build" || cfg.SystemPrompt != "environment-prompt" {
		t.Fatalf("config = %#v", cfg)
	}
	t.Setenv("YTEAM_BASE_URL", "")
	t.Setenv("YTEAM_MODEL", "")
	t.Setenv("YTEAM_AGENT", "")
	t.Setenv("YTEAM_SYSTEM_PROMPT", "")
	cfg, err = Load(root)
	if err != nil || cfg.BaseURL != "explicit" || cfg.Model != "explicit-model" || cfg.Agent != "plan" || cfg.SystemPrompt != "project-prompt" {
		t.Fatalf("file precedence = %#v, err=%v", cfg, err)
	}
}

func TestLoadRejectsMalformedExplicitConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTEAM_CONFIG", path)
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("expected malformed config error")
	}
}

func TestLoadAllowsEnvironmentToClearFileSecret(t *testing.T) {
	home := t.TempDir()
	data := []byte(`{"api_key":"file-secret","system_prompt":"file-prompt"}`)
	if err := os.WriteFile(filepath.Join(home, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTEAM_HOME", home)
	t.Setenv("YTEAM_CONFIG", "")
	t.Setenv("YTEAM_API_KEY", "")
	t.Setenv("YTEAM_SYSTEM_PROMPT", "")
	cfg, err := Load(t.TempDir())
	if err != nil || cfg.APIKey != "" || cfg.SystemPrompt != "file-prompt" {
		t.Fatalf("config = %#v, err=%v", cfg, err)
	}
}
