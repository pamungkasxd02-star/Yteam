package lsp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerOperationValidationAllowsDiagnosticsWithoutPosition(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	if _, err := manager.Execute(context.Background(), OperationInput{Operation: "diagnostics", FilePath: "main.go"}, root); err == nil || err.Error() != "no connected LSP server" {
		t.Fatalf("diagnostics validation = %v", err)
	}
	if _, err := manager.Execute(context.Background(), OperationInput{Operation: "codeAction", FilePath: "main.go", Line: 1, Character: 1, EndLine: 0, EndCharacter: 0}, root); err == nil || err.Error() != "no connected LSP server" {
		t.Fatalf("code action validation = %v", err)
	}
	if _, err := manager.Execute(context.Background(), OperationInput{Operation: "incomingCalls"}, root); err == nil || err.Error() != "call hierarchy item is required" {
		t.Fatalf("incoming validation = %v", err)
	}
	if _, err := manager.Execute(context.Background(), OperationInput{Operation: "hover", FilePath: "main.go", Line: 0, Character: 1}, root); err == nil {
		t.Fatal("expected position validation")
	}
	if !errors.Is(manager.Disconnect("missing"), nil) {
		t.Fatal("disconnect should be idempotent")
	}
}

func TestNormalizeExtensionsAndSelectClient(t *testing.T) {
	manager := NewManager()
	goClient := &Client{}
	typeScriptClient := &Client{}
	root := t.TempDir()
	manager.clients["go"] = clientEntry{client: goClient, root: root, extensions: map[string]bool{".go": true}}
	manager.clients["ts"] = clientEntry{client: typeScriptClient, root: root, extensions: map[string]bool{".ts": true}}
	if got := manager.selectClient(filepath.Join(root, "file.ts"), root); got != typeScriptClient {
		t.Fatal("extension-specific client was not selected")
	}
	if got := manager.selectClient("file.unknown", ""); got == nil {
		t.Fatal("fallback client was not selected")
	}
	got := normalizeExtensions([]string{"GO", ".go", " ts ", ""})
	if len(got) != 2 || got[0] != ".go" || got[1] != ".ts" {
		t.Fatalf("extensions = %#v", got)
	}
}
