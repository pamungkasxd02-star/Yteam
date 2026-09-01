package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/event"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func testServer(t *testing.T, token string) *Server {
	t.Helper()
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, BaseURL: "http://127.0.0.1:1", Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	j, err := event.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	app.AttachEvents(j)
	return New(app, j, token)
}

func TestHealthAndAuthorization(t *testing.T) {
	handler := testServer(t, "secret").Handler()
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/health", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("health = %d", r.Code)
	}
	r = httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized = %d", r.Code)
	}
	r = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK {
		t.Fatalf("authorized = %d", r.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "test" {
		t.Fatalf("body = %#v", body)
	}
}

func TestSessionListAndExport(t *testing.T) {
	s := testServer(t, "")
	id := s.Runtime.CurrentSession().ID
	handler := s.Handler()
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), id) {
		t.Fatalf("list = %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/session/"+id+"/export?format=json", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"messages"`) {
		t.Fatalf("export = %d %s", r.Code, r.Body.String())
	}
}

func TestInputAdmissionAndPromotionAreSeparate(t *testing.T) {
	s := testServer(t, "")
	id := s.Runtime.CurrentSession().ID
	handler := s.Handler()
	body := strings.NewReader(`{"content":"steer me","delivery":"steer"}`)
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session/"+id+"/input", body)
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusAccepted || !strings.Contains(r.Body.String(), "steer me") {
		t.Fatalf("admit = %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/session/"+id+"/input", nil)
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "steer me") {
		t.Fatalf("pending = %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/session/"+id+"/input/promote", nil)
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "steer me") {
		t.Fatalf("promote = %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/session/"+id+"/input", nil)
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK || strings.Contains(r.Body.String(), "steer me") {
		t.Fatalf("pending after promote = %d %s", r.Code, r.Body.String())
	}
}

func TestSessionMessagesContextAndEventReplay(t *testing.T) {
	s := testServer(t, "")
	id := s.Runtime.CurrentSession().ID
	if err := s.Runtime.Store.Append(id, session.Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Events.Publish(context.Background(), schema.EventPromptAdmitted, id, map[string]any{"content": "hello"}); err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/session/"+id+"/message", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "hello") {
		t.Fatalf("messages = %d %s", r.Code, r.Body.String())
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := http.Get(server.URL + "/api/session/" + id + "/event?after=0")
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	var stream strings.Builder
	for i := 0; i < 4; i++ {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			t.Fatal(readErr)
		}
		stream.WriteString(line)
		if strings.Contains(stream.String(), schema.EventPromptAdmitted) {
			break
		}
		if readErr == io.EOF {
			break
		}
	}
	_ = response.Body.Close()
	if !strings.Contains(stream.String(), schema.EventPromptAdmitted) {
		t.Fatalf("events = %s", stream.String())
	}
}
