package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestTUIStashCommandStoresAndClearsCurrentPrompt(t *testing.T) {
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
	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)
	ui.editor.Set("draft prompt")
	ui.promptParts = []schema.MessagePart{{Type: "text", Text: "expanded"}}
	if handled, err := ui.command(context.Background(), "/stash"); !handled || err != nil {
		t.Fatalf("stash handled=%v err=%v", handled, err)
	}
	entries := ui.promptStash.Entries()
	if len(entries) != 1 || entries[0].Input != "draft prompt" || ui.editor.String() != "" {
		t.Fatalf("stash entries=%#v editor=%q", entries, ui.editor.String())
	}
	if err := ui.popStash(); err != nil {
		t.Fatal(err)
	}
	if ui.editor.String() != "draft prompt" || len(ui.promptParts) != 1 {
		t.Fatalf("restored editor=%q parts=%#v", ui.editor.String(), ui.promptParts)
	}
}

func TestTUIStashDeleteRequiresTwoPresses(t *testing.T) {
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
	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)
	if err := ui.promptStash.Push(StashEntry{Input: "one", Timestamp: 1}); err != nil {
		t.Fatal(err)
	}
	if err := ui.promptStash.Push(StashEntry{Input: "two\nmore\nlines", Timestamp: 2}); err != nil {
		t.Fatal(err)
	}
	if err := ui.openStashPicker(); err != nil {
		t.Fatal(err)
	}
	// First delete press only arms the confirmation, does not remove.
	if err := ui.toggleStashDelete(); err != nil {
		t.Fatal(err)
	}
	if len(ui.promptStash.Entries()) != 2 {
		t.Fatalf("first press removed entry, got %d", len(ui.promptStash.Entries()))
	}
	if ui.stashDelete == -1 {
		t.Fatal("first press did not arm delete confirmation")
	}
	// Second press on the same entry removes it.
	if err := ui.toggleStashDelete(); err != nil {
		t.Fatal(err)
	}
	if len(ui.promptStash.Entries()) != 1 {
		t.Fatalf("second press did not remove entry, got %d", len(ui.promptStash.Entries()))
	}
}

func TestTUIStashPickerShowsLineCountFooter(t *testing.T) {
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
	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)
	if err := ui.promptStash.Push(StashEntry{Input: "single line", Timestamp: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := ui.promptStash.Push(StashEntry{Input: "multi\nline\nbody", Timestamp: 2000}); err != nil {
		t.Fatal(err)
	}
	if err := ui.openStashPicker(); err != nil {
		t.Fatal(err)
	}
	// Most recent first: "multi..." is entry index 1, rendered first.
	items := ui.picker.Items
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if !strings.Contains(items[0].Description, "~3 lines") {
		t.Fatalf("first item description = %q, want ~3 lines footer", items[0].Description)
	}
	if strings.Contains(items[1].Description, "lines") {
		t.Fatalf("single-line item description = %q, want no lines footer", items[1].Description)
	}
}
