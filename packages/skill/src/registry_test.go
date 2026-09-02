package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverReadsDescriptionFromFrontmatter(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "example")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := "---\nname: example\ndescription: A concise description\n---\n\n# Example\n\nBody"
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Description != "A concise description" || items[0].Name != "example" || items[0].Body != "# Example\n\nBody" {
		t.Fatalf("skills = %#v", items)
	}
}
