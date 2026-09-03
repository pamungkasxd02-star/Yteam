package codemode

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DependencyGraph maps packages and symbols across a codebase.
type DependencyGraph struct {
	Packages map[string][]string `json:"packages"`
	Symbols  map[string][]string `json:"symbols"`
}

// AnalyzeDirectory inspects all Go source files in a project root to build an AST dependency graph.
func AnalyzeDirectory(root string) (*DependencyGraph, error) {
	graph := &DependencyGraph{
		Packages: make(map[string][]string),
		Symbols:  make(map[string][]string),
	}

	fset := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "node_modules" || info.Name() == ".yteam" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return nil
		}

		_ = node.Name.Name
		var imports []string
		for _, imp := range node.Imports {
			cleanPath := strings.Trim(imp.Path.Value, `"`)
			imports = append(imports, cleanPath)
		}

		relPath, _ := filepath.Rel(root, path)
		graph.Packages[relPath] = imports
		return nil
	})

	if err != nil {
		return nil, err
	}

	for k, list := range graph.Packages {
		sort.Strings(list)
		graph.Packages[k] = list
	}

	return graph, nil
}
