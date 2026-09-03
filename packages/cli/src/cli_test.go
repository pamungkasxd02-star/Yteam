package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDispatchVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	err := Dispatch(context.Background(), []string{"version"}, bytes.NewReader(nil), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "YTEAM") {
		t.Fatalf("expected version string, got %q", out.String())
	}
}

func TestDispatchUnknown(t *testing.T) {
	var out, errOut bytes.Buffer
	err := Dispatch(context.Background(), []string{"foobar"}, bytes.NewReader(nil), &out, &errOut)
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}
