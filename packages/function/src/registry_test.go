package function

import (
	"context"
	"testing"
)

func TestFunctionRegistry(t *testing.T) {
	reg := NewRegistry()
	reg.Register("echo", func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{"echo": args["msg"]}, nil
	})

	res, err := reg.Call(context.Background(), "echo", map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.(map[string]any)
	if !ok || m["echo"] != "hello" {
		t.Fatalf("unexpected result: %#v", res)
	}

	jsonRes, err := reg.CallJSON(context.Background(), "echo", []byte(`{"msg":"world"}`))
	if err != nil || jsonRes != `{"echo":"world"}` {
		t.Fatalf("unexpected JSON result: %q, err=%v", jsonRes, err)
	}
}
