package runner

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestRunOptionsKeepConcurrentSessionOutputSeparate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		answer := "session-a"
		if strings.Contains(string(body), "session-b") {
			answer = "session-b"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"`+answer+`"}}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	store, err := session.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	one, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	runner := &Runner{Provider: provider.New(server.URL, ""), Store: store, MaxSteps: 1}
	var first, second string
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if err := runner.RunWithOptions(context.Background(), one, "test", "session-a", RunOptions{OnText: func(value string) { first += value }}); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer group.Done()
		if err := runner.RunWithOptions(context.Background(), two, "test", "session-b", RunOptions{OnText: func(value string) { second += value }}); err != nil {
			t.Error(err)
		}
	}()
	group.Wait()
	if first != "session-a" || second != "session-b" {
		t.Fatalf("outputs = %q, %q", first, second)
	}
}
