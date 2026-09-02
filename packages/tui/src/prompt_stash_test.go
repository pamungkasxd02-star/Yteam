package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestPromptStashPersistsTrimsPopsAndPreservesParts(t *testing.T) {
	home := t.TempDir()
	stash, err := OpenPromptStash(home)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < MaxStashEntries+2; index++ {
		if err := stash.Push(StashEntry{Input: "draft-" + string(rune('a'+index)), Parts: []schema.MessagePart{{Type: "file", Filename: "note.md"}}}); err != nil {
			t.Fatal(err)
		}
	}
	entries := stash.Entries()
	if len(entries) != MaxStashEntries {
		t.Fatalf("entries = %d, want %d", len(entries), MaxStashEntries)
	}
	last, ok, err := stash.Pop()
	if err != nil || !ok || len(last.Parts) != 1 || last.Parts[0].Filename != "note.md" {
		t.Fatalf("pop = %#v, %v, %v", last, ok, err)
	}
	reloaded, err := OpenPromptStash(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Entries()) != MaxStashEntries-1 {
		t.Fatalf("reloaded entries = %d", len(reloaded.Entries()))
	}
	if err := os.WriteFile(filepath.Join(home, "prompt-stash.jsonl"), []byte("bad-json\n{\"input\":\"valid\",\"timestamp\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err = OpenPromptStash(home)
	if err != nil || len(reloaded.Entries()) != 1 || reloaded.Entries()[0].Input != "valid" {
		t.Fatalf("malformed replay = %#v, err=%v", reloaded.Entries(), err)
	}
}
