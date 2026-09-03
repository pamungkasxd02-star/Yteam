package codemode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeDirectoryGraph(t *testing.T) {
	root := t.TempDir()
	f1 := filepath.Join(root, "a.go")
	_ = os.WriteFile(f1, []byte(`package a
import "fmt"
import "strings"
`), 0o600)

	graph, err := AnalyzeDirectory(root)
	if err != nil {
		t.Fatal(err)
	}

	imports, ok := graph.Packages["a.go"]
	if !ok || len(imports) != 2 {
		t.Fatalf("unexpected package graph: %#v", graph)
	}
	if imports[0] != "fmt" || imports[1] != "strings" {
		t.Fatalf("unexpected imports: %#v", imports)
	}
}
