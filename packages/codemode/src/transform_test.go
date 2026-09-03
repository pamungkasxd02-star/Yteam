package codemode

import (
	"strings"
	"testing"
)

func TestCodemodeASTTransform(t *testing.T) {
	src := `package main

func greet(user string) string {
	return "hello " + user
}
`
	res, err := RenameIdent(src, "user", "username")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Replacements < 2 {
		t.Fatalf("expected transform changes: %#v", res)
	}
	if !strings.Contains(res.Code, "func greet(username string)") {
		t.Fatalf("rename failed: %s", res.Code)
	}
}
