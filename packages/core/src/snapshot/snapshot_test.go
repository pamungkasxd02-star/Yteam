package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureDiffAndRestore(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "old.txt"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "ignored"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}

	service, err := New(home, root)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service.Capture()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Manifest.Entries) != 3 {
		t.Fatalf("entries = %#v", saved.Manifest.Entries)
	}

	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "added.txt"), []byte("added\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "nested", "old.txt")); err != nil {
		t.Fatal(err)
	}
	diff, err := service.Diff(saved.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if diff != "A added.txt\nM keep.txt\nD nested/old.txt\n" {
		t.Fatalf("diff = %q", diff)
	}

	if err := service.Restore(saved.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "keep.txt")); err != nil || string(got) != "before\n" {
		t.Fatalf("restored keep = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "nested", "old.txt")); err != nil || string(got) != "old\n" {
		t.Fatalf("restored old = %q, err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "added.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("added file remains, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "ignored")); err != nil {
		t.Fatalf("ignored file changed: %v", err)
	}
	if err := service.Remove(saved.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Load(saved.Manifest.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load after remove = %v", err)
	}
}

func TestManifestPathsCannotEscapeProject(t *testing.T) {
	service, err := New(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside", `..\\outside`, "/absolute", "", "."} {
		if _, err := service.safePath(path); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("safePath(%q) = %v", path, err)
		}
	}
	if _, err := service.safePath("nested/file.txt"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filepath.ToSlash(service.Root()), "\\") && service.Root() == "" {
		t.Fatal("unreachable portability guard")
	}
}
