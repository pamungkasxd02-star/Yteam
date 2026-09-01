package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestReadWriteEditAndGlobStayInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := Builtins(permission.New([]permission.Rule{
		{Action: "*", Resource: "*", Effect: permission.Allow},
	}))

	read, err := registry.Execute(context.Background(), call("read", `{"path":"note.txt"}`), Context{Root: root})
	if err != nil || read != "hello world" {
		t.Fatalf("read = %q, err = %v", read, err)
	}
	if _, err := registry.Execute(context.Background(), call("edit", `{"path":"note.txt","old":"hello","new":"hi"}`), Context{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), call("write", `{"path":"new.txt","content":"created"}`), Context{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), call("glob", `{"pattern":"*.txt"}`), Context{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), call("read", `{"path":"../outside.txt"}`), Context{Root: root}); err == nil {
		t.Fatal("expected outside-root rejection")
	}
}

func call(name, args string) schema.ToolCall {
	return schema.ToolCall{ID: "call_test", Type: "function", Name: name, Arguments: args}
}
