package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestSkillsBecomeSystemContext(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	skillDir := filepath.Join(root, "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Use the demo workflow."), 0o600); err != nil {
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
	app := New(config.Config{Home: home, Model: "test", SystemPrompt: "Base instructions."}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	items, err := app.Skills()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !strings.Contains(app.SystemPrompt(), "Use the demo workflow.") {
		t.Fatalf("skills = %#v, prompt = %q", items, app.SystemPrompt())
	}
}
