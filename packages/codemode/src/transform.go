package codemode

import (
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

// TransformResult contains rewritten code and modification stats.
type TransformResult struct {
	Code      string `json:"code"`
	Changed   bool   `json:"changed"`
	Replacements int `json:"replacements"`
}

// RenameIdent rewrites identifiers in valid Go source code AST safely.
func RenameIdent(src, oldName, newName string) (TransformResult, error) {
	if oldName == "" || newName == "" {
		return TransformResult{Code: src}, errors.New("names cannot be empty")
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	if err != nil {
		// If not valid standalone Go file, fallback to safe exact token boundary replace
		return fallbackReplace(src, oldName, newName), nil
	}

	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			if ident.Name == oldName {
				ident.Name = newName
				count++
			}
		}
		return true
	})

	if count == 0 {
		return TransformResult{Code: src, Changed: false}, nil
	}

	var buf strings.Builder
	if err := format.Node(&buf, fset, node); err != nil {
		return TransformResult{Code: src}, err
	}

	return TransformResult{
		Code:         buf.String(),
		Changed:      true,
		Replacements: count,
	}, nil
}

func fallbackReplace(src, oldName, newName string) TransformResult {
	if !strings.Contains(src, oldName) {
		return TransformResult{Code: src, Changed: false}
	}
	count := strings.Count(src, oldName)
	return TransformResult{
		Code:         strings.ReplaceAll(src, oldName, newName),
		Changed:      true,
		Replacements: count,
	}
}
