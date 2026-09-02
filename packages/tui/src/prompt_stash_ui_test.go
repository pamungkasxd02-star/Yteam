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
