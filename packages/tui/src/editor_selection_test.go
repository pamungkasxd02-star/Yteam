package tui

import "testing"

func TestEditorShiftMoveSetsAnchor(t *testing.T) {
	e := NewEditor()
	e.Set("abcdef")
	// cursor starts at end (6); move to 2 via Left x4
	for i := 0; i < 4; i++ {
		e.Left()
	}
	if e.Cursor() != 2 {
		t.Fatalf("cursor = %d, want 2", e.Cursor())
	}
	if e.Anchor() != -1 {
		t.Fatalf("initial anchor = %d, want -1", e.Anchor())
	}
	e.ShiftMove(e.Right) // select "c"
	if e.Anchor() != 2 {
		t.Fatalf("anchor = %d, want 2", e.Anchor())
	}
	s, en, ok := e.SelectionRange()
	if !ok || s != 2 || en != 3 {
		t.Fatalf("selection = %d-%d ok=%v, want 2-3 true", s, en, ok)
	}
	e.ShiftMove(e.Right) // select "cd"
	s, en, _ = e.SelectionRange()
	if s != 2 || en != 4 {
		t.Fatalf("selection = %d-%d, want 2-4", s, en)
	}
}

func TestEditorSelectAllAndDelete(t *testing.T) {
	e := NewEditor()
	e.Set("hello world")
	e.SelectAll()
	s, en, ok := e.SelectionRange()
	if !ok || s != 0 || en != 11 {
		t.Fatalf("select-all = %d-%d ok=%v, want 0-11", s, en, ok)
	}
	if !e.DeleteSelection() {
		t.Fatal("DeleteSelection returned false")
	}
	if e.String() != "" {
		t.Fatalf("after delete selection = %q, want empty", e.String())
	}
	if e.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0", e.Cursor())
	}
	if e.Anchor() != -1 {
		t.Fatalf("anchor = %d, want -1 after delete", e.Anchor())
	}
}

func TestEditorDeleteSelectionForward(t *testing.T) {
	e := NewEditor()
	e.Set("abcdef")
	// cursor at 6; move to 1 via Left x5
	for i := 0; i < 5; i++ {
		e.Left()
	}
	e.ShiftMove(e.Right)
	e.ShiftMove(e.Right) // select "bc"
	if !e.DeleteSelection() {
		t.Fatal("DeleteSelection returned false")
	}
	if e.String() != "adef" {
		t.Fatalf("after delete selection = %q, want %q", e.String(), "adef")
	}
}

func TestEditorSelectionClearedOnMove(t *testing.T) {
	e := NewEditor()
	e.Set("abcdef")
	// cursor at 6; move to 0 via Left x6
	for i := 0; i < 6; i++ {
		e.Left()
	}
	e.ShiftMove(e.Right) // anchor 0, selection "a"
	if e.Anchor() != 0 {
		t.Fatalf("anchor = %d, want 0", e.Anchor())
	}
	e.Right()
	e.ClearAnchor()
	if e.Anchor() != -1 {
		t.Fatalf("anchor after move = %d, want -1", e.Anchor())
	}
}
