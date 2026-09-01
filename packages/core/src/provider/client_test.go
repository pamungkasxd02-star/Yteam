package provider

import (
	"context"
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
