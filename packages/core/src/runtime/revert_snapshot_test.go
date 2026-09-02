package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptSnapshotCanRestoreProjectThroughRevert(t *testing.T) {
	r := testRuntime(t)
	path := filepath.Join(r.Root, "tracked.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No provider request is needed: the snapshot is captured before the user
	// message is persisted, and a cancelled run still leaves that checkpoint.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = r.Prompt(ctx, "make a change", discardWriter{})
	loaded, err := r.Store.Load(r.CurrentSession().ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].SnapshotID == "" {
		t.Fatalf("messages = %#v", loaded.Messages)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Root, "new.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StageRevert(loaded.Messages[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitRevert(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "before\n" {
		t.Fatalf("tracked = %q, err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new file remains, err=%v", err)
	}
	if loaded.Messages[0].Role != "user" {
		t.Fatal("unexpected message role")
	}
}

type discardWriter struct{}

func (discardWriter) Write(data []byte) (int, error) { return len(data), nil }
