package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRootFindsGitDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "src", "deep")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRoot(child)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}
