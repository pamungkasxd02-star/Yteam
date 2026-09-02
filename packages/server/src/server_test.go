package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestProviderUsageEndpoint(t *testing.T) {
	s := testServer(t, "")
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/provider/usage", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"total"`) || !strings.Contains(r.Body.String(), `"by_model"`) {
		t.Fatalf("usage = %d %s", r.Code, r.Body.String())
	}
}

func TestRunStateEndpoint(t *testing.T) {
	s := testServer(t, "")
	id := s.Runtime.CurrentSession().ID
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/session/"+id+"/run", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"session_id"`) || !strings.Contains(r.Body.String(), `"status"`) {
		t.Fatalf("run = %d %s", r.Code, r.Body.String())
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

func TestCompactAndRevertEndpoints(t *testing.T) {
	s := testServer(t, "")
	id := s.Runtime.CurrentSession().ID
	if err := s.Runtime.Store.Append(id, session.Message{Role: "user", Content: "change"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Runtime.Store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	messageID := loaded.Messages[0].ID
	handler := s.Handler()
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session/"+id+"/revert/stage", strings.NewReader(`{"message_id":"`+messageID+`","diff":"diff"}`))
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), messageID) {
		t.Fatalf("stage = %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/session/"+id+"/revert/clear", nil)
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK || strings.Contains(r.Body.String(), `"revert"`) {
		t.Fatalf("clear = %d %s", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/session/"+id+"/compact", strings.NewReader(`{"summary":"keep this","keep":1}`))
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "keep this") {
		t.Fatalf("compact = %d %s", r.Code, r.Body.String())
	}
}

func TestSnapshotEndpoints(t *testing.T) {
	s := testServer(t, "")
	id := s.Runtime.CurrentSession().ID
	path := filepath.Join(s.Runtime.Root, "snapshot.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := s.Handler()
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session/"+id+"/snapshot", nil)
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusCreated {
		t.Fatalf("snapshot = %d %s", r.Code, r.Body.String())
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &manifest); err != nil || manifest.ID == "" {
		t.Fatalf("manifest = %s, err=%v", r.Body.String(), err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/session/"+id+"/snapshot/"+manifest.ID, nil)
	handler.ServeHTTP(r, req)
	if r.Code != http.StatusOK || r.Body.String() != "M snapshot.txt\n" {
		t.Fatalf("snapshot diff = %d %q", r.Code, r.Body.String())
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

func TestMissingSessionIsNotFoundAndDeleteHasEmptyBody(t *testing.T) {
	s := testServer(t, "")
	handler := s.Handler()
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/session/ses_missing", nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("missing session = %d", r.Code)
	}
	id := s.Runtime.CurrentSession().ID
	r = httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest(http.MethodDelete, "/api/session/"+id, nil))
	if r.Code != http.StatusNoContent || r.Body.Len() != 0 {
		t.Fatalf("delete = %d body=%q", r.Code, r.Body.String())
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
