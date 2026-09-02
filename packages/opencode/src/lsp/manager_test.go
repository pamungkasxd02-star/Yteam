package lsp

import (
	"context"
	"testing"
)

func TestManagerStartsEmpty(t *testing.T) {
	if got := NewManager().Status(); len(got) != 0 {
		t.Fatalf("status = %#v", got)
	}
}

func TestManagerExecuteValidatesOperationAndPosition(t *testing.T) {
	m := NewManager()
	if _, err := m.Execute(context.Background(), OperationInput{Operation: "unknown"}, t.TempDir()); err == nil {
		t.Fatal("expected unsupported operation")
	}
	if _, err := m.Execute(context.Background(), OperationInput{Operation: "hover", FilePath: "missing.go", Line: 0, Character: 0}, t.TempDir()); err == nil {
		t.Fatal("expected invalid position")
	}
	if _, err := m.Execute(context.Background(), OperationInput{Operation: "workspaceSymbol"}, t.TempDir()); err == nil {
		t.Fatal("expected no-client error")
	}
}
