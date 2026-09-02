package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigsFromHome(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "plugins.json"), []byte(`{"plugins":{"demo":{"command":["demo-plugin"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTEAM_PLUGIN_CONFIG", "")
	configs, err := LoadConfigs(home)
	if err != nil || len(configs) != 1 || configs["demo"].Command[0] != "demo-plugin" {
		t.Fatalf("configs=%#v err=%v", configs, err)
	}
}
