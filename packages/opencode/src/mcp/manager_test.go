package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestManagerStartsEmpty(t *testing.T) {
	if got := NewManager().Status(); len(got) != 0 {
		t.Fatalf("status = %#v", got)
	}
}

func TestToolNameAndInputSchemaMatchCatalogNormalization(t *testing.T) {
	if got := ToolName("server:name", "read.file/v2"); got != "server_name_read_file_v2" {
		t.Fatalf("tool name = %q", got)
	}
	normalized := NormalizeInputSchema(map[string]any{"description": "input"})
	if normalized["type"] != "object" || normalized["additionalProperties"] != false {
		t.Fatalf("schema = %#v", normalized)
	}
	if _, ok := normalized["properties"].(map[string]any); !ok {
		t.Fatalf("properties = %#v", normalized["properties"])
	}
}

func TestManagerRemoteInitializesAndRegistersTools(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		result := map[string]any{}
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05"}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "remote_read", "description": "read"}}}
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	defer server.Close()
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	manager := NewManager()
	if err := manager.ConnectRemote(context.Background(), app, "docs", RemoteConfig{URL: server.URL}); err != nil {
		t.Fatal(err)
	}
	tools := app.RunnerTools()
	found := false
	for _, tool := range tools {
		if tool.Function.Name == "docs_remote_read" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tools = %#v", tools)
	}
	if len(methods) != 2 || methods[0] != "initialize" || methods[1] != "tools/list" {
		t.Fatalf("methods = %#v", methods)
	}
}
