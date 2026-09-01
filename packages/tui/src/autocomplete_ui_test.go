package tui

import (
	"bytes"
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
