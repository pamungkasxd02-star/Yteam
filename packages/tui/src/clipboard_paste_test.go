package tui

import (
	"bytes"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestClipboardPasteUsesPromptPastePipeline(t *testing.T) {
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
	ui.clipboardRead = func() (string, error) { return "one\ntwo\nthree", nil }
	if err := ui.pasteFromClipboard(); err != nil {
		t.Fatal(err)
	}
	if ui.editor.String() != "[Pasted ~3 lines] " || len(ui.promptParts) != 1 {
		t.Fatalf("clipboard prompt = %q parts=%#v", ui.editor.String(), ui.promptParts)
	}
}
