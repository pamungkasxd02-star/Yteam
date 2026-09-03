package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "YTEAM Web Interface") {
		t.Fatalf("unexpected body: %s", body)
	}
}
