package command

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHintsAndExpandMatchCommandTemplateBehavior(t *testing.T) {
	template := "review $2 then $1; remaining: $ARGUMENTS"
	if got := Hints(template); len(got) != 3 || got[0] != "$1" || got[1] != "$2" || got[2] != "$ARGUMENTS" {
		t.Fatalf("hints = %#v", got)
	}
	if got := Expand(template, []string{"first", "second", "extra"}); got != "review second then first; remaining: first second extra" {
		t.Fatalf("expanded = %q", got)
	}
}

func TestDiscoverMarkdownCommandsAndFrontmatter(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, ".opencode", "commands", "team")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "---\ndescription: Team review\nagent: plan\nmodel: review-model\nvariant: precise\nsubtask: true\n---\nReview $ARGUMENTS"
	if err := os.WriteFile(filepath.Join(directory, "check.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var found Info
	for _, item := range items {
		if item.Name == "team/check" {
			found = item
		}
	}
	if found.Name == "" || found.Description != "Team review" || found.Agent != "plan" || found.Model != "review-model" || found.Variant != "precise" || !found.Subtask || len(found.Hints) != 1 || found.Hints[0] != "$ARGUMENTS" {
		t.Fatalf("command = %#v", found)
	}
}
