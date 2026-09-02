package tui

import "testing"

func TestSeparatorWidthIsClampedToPositiveTerminalWidth(t *testing.T) {
	for _, width := range []int{0, 1, 20, 80} {
		if got := maxInt(width, 1); got < 1 {
			t.Fatalf("width %d produced invalid separator width %d", width, got)
		}
	}
}
