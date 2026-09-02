package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestClientUsesBearerAndDecodesTypedResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"healthy": true})
		case "/api/session":
			_ = json.NewEncoder(w).Encode([]session.Session{{ID: "ses_test", Title: "Test"}})
		case "/api/session/ses_test":
			_ = json.NewEncoder(w).Encode(session.Session{ID: "ses_test", Title: "Test"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	api := New(server.URL, "secret")
	health, err := api.Health(context.Background())
	if err != nil || health["healthy"] != true {
		t.Fatalf("health = %#v, err=%v", health, err)
	}
	sessions, err := api.Sessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ID != "ses_test" {
		t.Fatalf("sessions = %#v, err=%v", sessions, err)
	}
	item, err := api.Session(context.Background(), "ses_test")
	if err != nil || item.ID != "ses_test" {
		t.Fatalf("session = %#v, err=%v", item, err)
	}
}

func TestEventStreamDecodesSSEAndCloses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": keepalive\nevent: message.text.delta\ndata: {\"type\":\"message.text.delta\",\ndata: \"bad\"}\n\n"))
	}))
	defer server.Close()
	api := New(server.URL, "")
	stream, err := api.Events(context.Background(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	event, err := stream.Next()
	if err == nil {
		t.Fatalf("event = %#v, err=%v", event, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("closed stream err = %v", err)
	}
}

func TestEventStreamDecodesCommentAndMultilineData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": keepalive\nevent: test\ndata: {\"type\":\"test\",\ndata: \"sequence\":3}\n\n"))
	}))
	defer server.Close()
	api := New(server.URL, "")
	stream, err := api.Events(context.Background(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	event, err := stream.Next()
	if err != nil || event.Type != "test" || event.Sequence != 3 {
		t.Fatalf("event = %#v, err=%v", event, err)
	}
	if _, err := stream.Next(); err == nil {
		t.Fatal("expected EOF after stream body")
	}
}
