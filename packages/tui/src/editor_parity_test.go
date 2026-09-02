package tui

import "testing"

func TestNormalizePromptContentOnlyRemovesSingleLineTrailingNewline(t *testing.T) {
	for _, item := range []struct {
		input string
		want  string
	}{
		{"hello\n", "hello"},
		{"hello\r\n", "hello"},
		{"hello\nworld\n", "hello\nworld\n"},
		{"hello", "hello"},
	} {
		if got := normalizePromptContent(item.input); got != item.want {
			t.Fatalf("normalize(%q) = %q, want %q", item.input, got, item.want)
		}
	}
}
