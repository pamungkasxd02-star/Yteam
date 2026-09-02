package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

func TestCompleteParsesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"halo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" dunia\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := New(server.URL, "secret")
	var got string
	err := client.Complete(context.Background(), protocol.ChatRequest{Model: "test"}, func(delta protocol.StreamDelta) error {
		got += delta.Content
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "halo dunia" {
		t.Fatalf("content = %q", got)
	}
}

func TestCompleteRecordsUsageAndFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "test-model",
			"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": "done"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	}))
	defer server.Close()
	client := New(server.URL, "")
	var delta protocol.StreamDelta
	if err := client.Complete(context.Background(), protocol.ChatRequest{Model: "test-model"}, func(value protocol.StreamDelta) error { delta = value; return nil }); err != nil {
		t.Fatal(err)
	}
	if delta.Content != "done" || delta.FinishReason != "stop" || delta.Usage == nil || delta.Usage.TotalTokens != 5 {
		t.Fatalf("delta = %#v", delta)
	}
	usage := client.Usage()
	if usage.Requests != 1 || usage.PromptTokens != 3 || usage.CompletionTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %#v", usage)
	}
}
