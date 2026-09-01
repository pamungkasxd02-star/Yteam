package lsp

import "testing"

func TestManagerStartsEmpty(t *testing.T) {
	if got := NewManager().Status(); len(got) != 0 {
		t.Fatalf("status = %#v", got)
	}
}
