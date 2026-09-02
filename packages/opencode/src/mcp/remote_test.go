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
