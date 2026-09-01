package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeCaller struct {
	name string
	args map[string]any
}

func (f *fakeCaller) CallTool(_ context.Context, name string, args map[string]any) (string, error) {
	f.name, f.args = name, args
	return "ok", nil
}

func TestExternalToolSeparatesDisplayAndRemoteNames(t *testing.T) {
	caller := &fakeCaller{}
	tool := ExternalTool{Caller: caller, ToolName: "server_read", RemoteName: "read"}
	result, err := tool.Execute(context.Background(), Context{}, json.RawMessage(`{"path":"file.txt"}`))
	if err != nil || result != "ok" {
		t.Fatalf("result = %q, err = %v", result, err)
	}
	if caller.name != "read" {
		t.Fatalf("remote tool name = %q", caller.name)
	}
	if caller.args["path"] != "file.txt" {
		t.Fatalf("args = %#v", caller.args)
	}
}
