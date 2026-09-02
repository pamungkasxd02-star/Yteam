package tui

import "testing"

func TestViewportResizeClampsShortTerminalHeight(t *testing.T) {
	v := NewViewport(80, 10)
	v.SetLines([]string{"one", "two"})
	v.SetSize(40, 0)
	if v.Width != 40 || v.Height != 1 {
		t.Fatalf("size = %dx%d, want 40x1", v.Width, v.Height)
	}
}

func TestViewportResizePreservesFollowBottomState(t *testing.T) {
	v := NewViewport(80, 3)
	v.SetLines([]string{"0", "1", "2", "3", "4"})
	if !v.FollowBottom || v.Offset != 2 {
		t.Fatalf("initial viewport = %#v", v)
	}
	v.SetSize(120, 2)
	if !v.FollowBottom || v.Offset != 3 {
		t.Fatalf("resized viewport = %#v", v)
	}
	v.Page(-1)
	v.SetSize(100, 4)
	if v.FollowBottom || v.Offset != 1 {
		t.Fatalf("manual viewport moved on resize = %#v", v)
	}
}
