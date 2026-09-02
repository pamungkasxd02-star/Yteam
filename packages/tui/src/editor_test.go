package tui

import "testing"

func TestEditorMultilineCursorMovement(t *testing.T) {
	e := NewEditor()
	e.Set("abc\ndef")
	e.Home()
	e.Up()
	if e.Cursor() != 0 {
		t.Fatalf("cursor at top = %d", e.Cursor())
	}
	e.End()
	if e.Cursor() != 3 {
		t.Fatalf("first line end = %d", e.Cursor())
	}
	e.Down()
	if e.Cursor() != 7 {
		t.Fatalf("second line end = %d", e.Cursor())
	}
	e.Backspace()
	if e.String() != "abc\nde" {
		t.Fatalf("value = %q", e.String())
	}
}

func TestEditorHistoryRestoresDraft(t *testing.T) {
	e := NewEditor()
	e.AddHistory("one")
	e.AddHistory("two")
	e.Set("draft")
	if !e.HistoryUp() || e.String() != "two" {
		t.Fatalf("up = %q", e.String())
	}
	if !e.HistoryUp() || e.String() != "one" {
		t.Fatalf("second up = %q", e.String())
	}
	if !e.HistoryDown() || e.String() != "two" {
		t.Fatalf("down = %q", e.String())
	}
	if !e.HistoryDown() || e.String() != "draft" {
		t.Fatalf("restore = %q", e.String())
	}
}

func TestSplitEditorCommandHonorsQuotesAndEscapes(t *testing.T) {
	parts, err := splitEditorCommand(`code --reuse-window "my notes.md"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--reuse-window", "my notes.md"}
	if len(parts) != len(want) {
		t.Fatalf("parts = %#v", parts)
	}
	for index := range want {
		if parts[index] != want[index] {
			t.Fatalf("part %d = %q, want %q", index, parts[index], want[index])
		}
	}
}

func TestSplitEditorCommandRejectsUnterminatedQuotes(t *testing.T) {
	if _, err := splitEditorCommand(`code "notes.md`); err == nil {
		t.Fatal("expected unterminated quote error")
	}
}
