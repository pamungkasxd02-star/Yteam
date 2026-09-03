package desktop

import (
	"strings"
	"testing"
)

func TestDesktopBridgePlatform(t *testing.T) {
	b := NewBridge()
	p := b.Platform()
	if !strings.Contains(p, "/") {
		t.Fatalf("unexpected platform format: %s", p)
	}
}
