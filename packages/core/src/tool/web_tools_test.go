package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebFetchTool(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock content"))
	}))
	defer ts.Close()

	tool := NewWebFetch()
	raw, _ := json.Marshal(map[string]string{"url": ts.URL})
	res, err := tool.Execute(context.Background(), Context{}, raw)
	if err != nil || res != "mock content" {
		t.Fatalf("fetch failed: res=%q, err=%v", res, err)
	}
}

func TestWebSearchTool(t *testing.T) {
	tool := NewWebSearch()
	if tool.Name() != "websearch" {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}
