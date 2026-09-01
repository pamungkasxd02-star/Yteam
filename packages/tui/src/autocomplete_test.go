package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutocompleteCommandsAndFiles(t *testing.T) {
	a := NewAutocomplete()
	a.Refresh("/mo", 3, t.TempDir())
	item, ok := a.Selected()
	if !ok || item.ID != "/models" {
		t.Fatalf("command = %#v, %v", item, ok)
	}
	e := NewEditor()
	e.Set("@src/ma")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.Refresh(e.String(), e.Cursor(), root)
	if !a.Visible {
		t.Fatal("file completion is not visible")
	}
	if !a.Accept(e) || e.String() != "@src\\main.go " {
		t.Fatalf("accepted = %q", e.String())
	}
}
