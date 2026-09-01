package config

import "testing"

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
}
