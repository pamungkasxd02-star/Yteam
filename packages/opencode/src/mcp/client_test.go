package mcp

import (
	"context"
	"testing"
)

func TestMCPRequiresCommand(t *testing.T) {
	if _, err := Start(context.Background(), Config{}); err == nil {
		t.Fatal("expected empty command error")
	}
}
