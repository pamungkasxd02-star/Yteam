package tui

import "testing"

func TestEditorDeleteToLineEnd(t *testing.T) {
	e := NewEditor()
	e.Set("hello\nworld")
	// cursor at end (11); move to start of "world" line.
	for i := 0; i < 5; i++ {
		e.Left()
	}
	// cursor now at 7 (the 'w' of world). Delete to end of line removes "world".
	e.DeleteToLineEnd()
	if e.String() != "hello\n" {
		t.Fatalf("after delete-to-line-end = %q, want %q", e.String(), "hello\n")
	}
}

func TestEditorDeleteToLineStart(t *testing.T) {
	e := NewEditor()
	e.Set("hello world")
	// cursor at end; move to position 5 (after "hello").
	for i := 0; i < 6; i++ {
		e.Left()
	}
	// cursor at 5. Delete to line start removes "hello".
	e.DeleteToLineStart()
	if e.String() != " world" {
		t.Fatalf("after delete-to-line-start = %q, want %q", e.String(), " world")
	}
	if e.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0", e.Cursor())
	}
}

func TestEditorUndoRedo(t *testing.T) {
	e := NewEditor()
	e.Set("abc")
	e.Insert("d") // "abcd"
	e.Backspace() // "abc"
	if e.String() != "abc" {
		t.Fatalf("after edits = %q, want abc", e.String())
	}
	e.Undo() // restore "abcd"
	if e.String() != "abcd" {
		t.Fatalf("after undo = %q, want abcd", e.String())
	}
	e.Undo() // restore "abc" (before Insert)
	if e.String() != "abc" {
		t.Fatalf("after undo2 = %q, want abc", e.String())
	}
	e.Redo() // "abcd" again
	if e.String() != "abcd" {
		t.Fatalf("after redo = %q, want abcd", e.String())
	}
}
