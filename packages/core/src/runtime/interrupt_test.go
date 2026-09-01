package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestInterruptSessionCancelsProviderRun(t *testing.T) {
	started := make(chan struct{})
	var startedOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		w.Header().Set("Content-Type", "text/event-stream")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
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
	app := New(config.Config{Home: home, BaseURL: server.URL, Model: "test"}, root, store, current, provider.New(server.URL, ""))
	done := make(chan error, 1)
	go func() { done <- app.Prompt(context.Background(), "cancel this", discard{}) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider request did not start")
	}
	app.InterruptSession(current.ID)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("prompt did not stop after interrupt")
	}
}

type discard struct{}

func (discard) Write(data []byte) (int, error) { return len(data), nil }
