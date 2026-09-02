package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteToolPaginationRejectsDuplicateCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"tools": []any{}, "nextCursor": "same"}})
	}))
	defer server.Close()
	remote, err := NewRemote(RemoteConfig{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.AllTools(context.Background()); err == nil {
		t.Fatal("expected duplicate cursor error")
	}
}

func TestRemoteFallsBackToSSEAndCallsTool(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			http.Error(w, "streamable unsupported", http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"remote ok\"}]}}\n\n"))
	}))
	defer server.Close()
	remote, err := NewRemote(RemoteConfig{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := remote.CallTool(context.Background(), "read", map[string]any{"path": "x"})
	if err != nil || !strings.Contains(result, "remote ok") {
		t.Fatalf("result = %q, err = %v", result, err)
	}
	if hits != 2 {
		t.Fatalf("requests = %d, want 2", hits)
	}
}

func TestRemoteCatalogListsPromptsResourcesAndTemplatesWithCapabilityGating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		result := map[string]any{}
		switch request.Method {
		case "initialize":
			result = map[string]any{"capabilities": map[string]any{"prompts": map[string]any{}, "resources": map[string]any{}}}
		case "prompts/list":
			result = map[string]any{"prompts": []any{map[string]any{"name": "p1"}}}
		case "resources/list":
			if request.Params["cursor"] == nil {
				result = map[string]any{"resources": []any{map[string]any{"uri": "file://one", "name": "one"}}, "nextCursor": "next"}
			} else {
				result = map[string]any{"resources": []any{map[string]any{"uri": "file://two", "name": "two"}}}
			}
		case "resources/templates/list":
			result = map[string]any{"resourceTemplates": []any{map[string]any{"uriTemplate": "file:///{path}", "name": "files"}}}
		default:
			result = map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	defer server.Close()
	remote, err := NewRemote(RemoteConfig{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	prompts, err := remote.AllPrompts(context.Background())
	if err != nil || len(prompts) != 1 || prompts[0].Name != "p1" {
		t.Fatalf("prompts=%#v err=%v", prompts, err)
	}
	resources, err := remote.AllResources(context.Background())
	if err != nil || len(resources) != 2 || resources[1].Name != "two" {
		t.Fatalf("resources=%#v err=%v", resources, err)
	}
	templates, err := remote.AllResourceTemplates(context.Background())
	if err != nil || len(templates) != 1 || templates[0].Name != "files" {
		t.Fatalf("templates=%#v err=%v", templates, err)
	}
	if !remote.Supports("prompts") || !remote.Supports("resources") || remote.Supports("logging") {
		t.Fatal("capability gating mismatch")
	}
}
