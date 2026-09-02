package tui

import "testing"

func TestDisplayClustersKeepCombiningMarksAndJoinedEmojiTogether(t *testing.T) {
	value := "e\u0301 👨‍👩‍👧‍👦 中文"
	units := displayUnits(value)
	if len(units) != 6 {
		t.Fatalf("clusters = %d, want 6: %#v", len(units), units)
	}
	if got := displayCharAt(value, 0); got != "e\u0301" {
		t.Fatalf("combining cluster = %q", got)
	}
	if got := displayCharAt(value, 3); got != "👨‍👩‍👧‍👦" {
		t.Fatalf("joined emoji = %q", got)
	}
	if got := displayWidth(value); got != 9 {
		t.Fatalf("display width = %d, want 9", got)
	}
}

func TestDisplaySliceUsesCellOffsets(t *testing.T) {
	value := "a界b"
	if got := displaySlice(value, 1, 3); got != "界" {
		t.Fatalf("slice = %q, want %q", got, "界")
	}
	if got := displayOffsetIndex(value, 4); got != len(value) {
		t.Fatalf("end byte = %d, want %d", got, len(value))
	}
}

func TestEditorMovesAndDeletesWholeGraphemeClusters(t *testing.T) {
	e := NewEditor()
	e.Set("e\u0301👨‍👩‍👧‍👦x")
	e.Left()
	if e.String() != "e\u0301👨‍👩‍👧‍👦x" || e.Cursor() != len([]rune("e\u0301👨‍👩‍👧‍👦")) {
		t.Fatalf("left cursor = %d", e.Cursor())
	}
	e.Backspace()
	if e.String() != "e\u0301x" {
		t.Fatalf("backspace split cluster: %q", e.String())
	}
	e.Home()
	e.Right()
	e.Backspace()
	if e.String() != "x" {
		t.Fatalf("combining backspace split cluster: %q", e.String())
	}
}
