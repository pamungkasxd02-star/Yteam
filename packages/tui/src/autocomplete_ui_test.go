package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestUIRefreshAutocompleteTracksEditorCursor(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	ui := New(app, bytes.NewBuffer(nil), &bytes.Buffer{})
	ui.editor.Set("/mod")
	ui.refreshAutocomplete()
	if ui.autocomplete == nil || !ui.autocomplete.Visible {
		t.Fatal("autocomplete did not open")
	}
	if ui.autocomplete.Query != "mod" {
		t.Fatalf("query = %q", ui.autocomplete.Query)
	}
	ui.editor.Set("plain")
	ui.refreshAutocomplete()
	if ui.autocomplete.Visible {
		t.Fatal("autocomplete stayed open for plain input")
	}
}

func TestAutocompleteIncludesDiscoveredCommands(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "commands"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "commands", "explain.md"), []byte("---\ndescription: Explain\n---\nExplain $ARGUMENTS"), 0o600); err != nil {
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
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	ui := New(app, bytes.NewBuffer(nil), &bytes.Buffer{})
	ui.editor.Set("/exp")
	ui.refreshAutocomplete()
	item, ok := ui.autocomplete.Selected()
	if !ok || item.ID != "/explain" {
		t.Fatalf("command suggestion = %#v, ok=%v", item, ok)
	}
}

func TestAutocompleteIncludesAgentsAndCreatesAgentPart(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	ui := New(app, bytes.NewBuffer(nil), &bytes.Buffer{})
	ui.editor.Set("@bu")
	ui.refreshAutocomplete()
	item, ok := ui.autocomplete.Selected()
	if !ok || item.ID != "build" {
		t.Fatalf("agent suggestion = %#v, ok=%v", item, ok)
	}
	ui.acceptAutocomplete()
	if ui.editor.String() != "@build " || len(ui.promptParts) != 1 || ui.promptParts[0].Type != "agent" {
		t.Fatalf("agent part = editor=%q parts=%#v", ui.editor.String(), ui.promptParts)
	}
}
