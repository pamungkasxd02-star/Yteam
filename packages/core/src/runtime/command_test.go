package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestRuntimeDiscoversProjectCommandRegistry(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	directory := filepath.Join(root, ".opencode", "commands")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "explain.md"), []byte("---\ndescription: Explain a target\nagent: plan\n---\nExplain $1 using $ARGUMENTS"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	r := New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	item, ok := r.Commands["explain"]
	if !ok || item.Description != "Explain a target" || item.Agent != "plan" || len(item.Hints) != 2 {
		t.Fatalf("commands = %#v", r.Commands)
	}
	list := r.CommandList()
	if len(list) < 3 || list[0].Name != "explain" {
		t.Fatalf("command list = %#v", list)
	}
}
