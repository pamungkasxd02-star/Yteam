package tui

import (
	"bytes"
	"context"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestPipeModeDispatchesCommandsBeforePrompt(t *testing.T) {
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
	if err := New(app, bytes.NewBufferString("  \ufeff /help  \n /quit\n"), &output).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(current.Messages) != 0 {
		t.Fatalf("command input became prompt messages: %#v", current.Messages)
	}
	if !bytes.Contains(output.Bytes(), []byte("YTEAM")) {
		t.Fatalf("help output missing: %q", output.String())
	}
}
