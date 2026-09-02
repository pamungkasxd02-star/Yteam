package tui

import "testing"

func TestWrapTextPreservesNewlinesAndDisplayWidth(t *testing.T) {
	lines := wrapText("ab界\nxy", 4)
	if len(lines) != 2 || lines[0] != "ab界" || lines[1] != "xy" {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestViewportFollowsBottomAndPagesAway(t *testing.T) {
	v := NewViewport(10, 2)
	v.SetLines([]string{"0", "1", "2", "3"})
	if got := v.Visible(); len(got) != 2 || got[0] != "2" || got[1] != "3" {
		t.Fatalf("bottom = %#v", got)
	}
	v.Page(-1)
	if got := v.Visible(); len(got) != 2 || got[0] != "0" || got[1] != "1" {
		t.Fatalf("page up = %#v", got)
	}
	v.SetLines([]string{"0", "1", "2", "3", "4"})
	if got := v.Visible(); got[0] != "0" {
		t.Fatalf("preserved offset = %#v", got)
	}
	v.ToBottom()
	if got := v.Visible(); got[0] != "3" {
		t.Fatalf("to bottom = %#v", got)
	}
}
