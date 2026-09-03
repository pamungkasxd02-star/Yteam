package containers

import (
	"testing"
)

func TestRunnerCreation(t *testing.T) {
	r := NewRunner("golang:1.22-alpine")
	if r.Image != "golang:1.22-alpine" {
		t.Fatalf("unexpected image: %s", r.Image)
	}

	def := NewRunner("")
	if def.Image != "alpine:latest" {
		t.Fatalf("unexpected default image: %s", def.Image)
	}
}
