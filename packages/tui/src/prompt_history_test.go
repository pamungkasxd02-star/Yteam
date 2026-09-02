package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPromptHistoryPersistsDeduplicatesTrimsAndReplays(t *testing.T) {
	home := t.TempDir()
	history, err := OpenPromptHistory(home)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MaxHistoryEntries+3; index++ {
		if err := history.Append("prompt-" + itoa(index)); err != nil {
			t.Fatal(err)
		}
	}
	if err := history.Append("prompt-52"); err != nil {
		t.Fatal(err)
	}
	entries := history.Entries()
	if len(entries) != MaxHistoryEntries || entries[len(entries)-1].Input != "prompt-52" {
		t.Fatalf("entries = %#v", entries)
	}
	reloaded, err := OpenPromptHistory(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Entries()) != MaxHistoryEntries {
		t.Fatalf("reloaded entries = %d", len(reloaded.Entries()))
	}
	if value, ok := reloaded.Move(-1, ""); !ok || value != "prompt-52" {
		t.Fatalf("latest = %q, %v", value, ok)
	}
	if value, ok := reloaded.Move(1, "prompt-52"); !ok || value != "" {
		t.Fatalf("draft = %q, %v", value, ok)
	}
	path := filepath.Join(home, "prompt-history.jsonl")
	if err := os.WriteFile(path, []byte("bad-json\n{\"input\":\"valid\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err = OpenPromptHistory(home)
	if err != nil || len(reloaded.Entries()) != 1 || reloaded.Entries()[0].Input != "valid" {
		t.Fatalf("malformed replay = %#v, err=%v", reloaded.Entries(), err)
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
