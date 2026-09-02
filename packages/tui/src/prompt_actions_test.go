package tui

import (
	"bytes"
	"context"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func newPromptActionUI(t *testing.T) *UI {
	t.Helper()
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
	return New(app, bytes.NewBuffer(nil), &bytes.Buffer{})
}

func TestClearPromptRetainsLongDraftAndParts(t *testing.T) {
	ui := newPromptActionUI(t)
	ui.editor.Set("this is a long draft worth retaining")
	ui.promptParts = []schema.MessagePart{{Type: "file", Filename: "notes.md"}}
	if err := ui.clearPrompt(); err != nil {
		t.Fatal(err)
	}
	if ui.editor.String() != "" || len(ui.promptParts) != 0 {
		t.Fatalf("prompt was not cleared: editor=%q parts=%#v", ui.editor.String(), ui.promptParts)
	}
	entries := ui.promptHistory.Entries()
	if len(entries) != 1 || entries[0].Input != "this is a long draft worth retaining" || len(entries[0].Parts) != 1 {
		t.Fatalf("retained entries = %#v", entries)
	}
}

func TestClearPromptDropsShortDraftWithoutParts(t *testing.T) {
	ui := newPromptActionUI(t)
	ui.editor.Set("short draft")
	if err := ui.clearPrompt(); err != nil {
		t.Fatal(err)
	}
	if len(ui.promptHistory.Entries()) != 0 || ui.editor.String() != "" {
		t.Fatalf("short draft was retained unexpectedly: %#v", ui.promptHistory.Entries())
	}
}

func TestClearPromptDoesNotRunWhenPickerOwnsFocus(t *testing.T) {
	ui := newPromptActionUI(t)
	ui.editor.Set("draft that should remain")
	ui.picker = NewPicker("Commands", []PickerItem{{ID: "/help", Label: "/help"}})
	key := ui.keymap.Normalize(Key{Kind: KeyCtrlC})
	if ui.picker == nil || key.Kind != KeyClear {
		t.Fatalf("setup key/picker invalid: key=%#v picker=%#v", key, ui.picker)
	}
	if err := ui.handlePickerKey(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if ui.editor.String() != "draft that should remain" {
		t.Fatalf("picker-owned clear modified prompt: %q", ui.editor.String())
	}
}
