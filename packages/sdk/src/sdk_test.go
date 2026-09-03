package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSDKClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "test-token")
	health, err := c.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if health["status"] != "ok" {
		t.Fatalf("unexpected health status: %v", health)
	}
}
