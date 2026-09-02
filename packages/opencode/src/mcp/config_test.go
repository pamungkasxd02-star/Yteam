package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRemoteConfigsFromHomeAndEnvironment(t *testing.T) {
	home := t.TempDir()
	data, err := json.Marshal(RemoteConfigFile{Servers: map[string]RemoteConfig{
		"docs": {URL: "https://example.invalid/mcp", Headers: map[string]string{"Authorization": "Bearer test"}, Timeout: time.Second},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "mcp.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTEAM_MCP_CONFIG", "")
	t.Setenv("YTEAM_MCP_URL", "")
	configs, err := LoadRemoteConfigs(home)
	if err != nil || configs["docs"].URL != "https://example.invalid/mcp" {
		t.Fatalf("file config = %#v, err=%v", configs, err)
	}
	t.Setenv("YTEAM_MCP_URL", "https://example.invalid/env")
	t.Setenv("YTEAM_MCP_HEADERS", `{"X-Test":"yes"}`)
	t.Setenv("YTEAM_MCP_TIMEOUT", "2s")
	configs, err = LoadRemoteConfigs(home)
	if err != nil || configs["default"].Headers["X-Test"] != "yes" || configs["default"].Timeout != 2*time.Second {
		t.Fatalf("environment config = %#v, err=%v", configs, err)
	}
}
