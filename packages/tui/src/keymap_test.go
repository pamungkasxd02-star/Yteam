package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeymapLoadsPortableOverrides(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "tui.json")
	if err := os.WriteFile(path, []byte(`{"keybinds":{"input.newline":"ctrl+n","input.word.backward":"alt+left","unknown.action":"x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	keymap, err := LoadKeymap(home)
	if err != nil {
		t.Fatal(err)
	}
	if keymap.Binding(ActionNewline) != "ctrl+n" || keymap.Binding(ActionWordBackward) != "alt+left" {
		t.Fatalf("bindings = %#v", keymap.bindings)
	}
	if keymap.Binding(ActionSubmit) != "return" {
		t.Fatalf("default submit = %q", keymap.Binding(ActionSubmit))
	}
}

func TestNormalizeKeyNameMatchesOpenCodeAliases(t *testing.T) {
	for input, want := range map[string]string{"enter": "return", "esc": "escape", "pgup": "pageup", "ctrl+esc": "ctrl+escape"} {
		if got := normalizeKeyName(input); got != want {
			t.Fatalf("normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestKeymapNormalizesConfiguredActions(t *testing.T) {
	keymap := DefaultKeymap()
	keymap.bindings[ActionWordBackward] = "ctrl+left"
	key := keymap.Normalize(Key{Kind: KeyWordLeft})
	if key.Kind != KeyWordLeft {
		t.Fatalf("normalized word key = %d", key.Kind)
	}
}

func TestKeymapCanRemapExit(t *testing.T) {
	keymap := DefaultKeymap()
	keymap.bindings[ActionExit] = "ctrl+q"
	if got := keymap.Normalize(Key{Kind: KeyCtrlQ}); got.Kind != KeyCtrlC {
		t.Fatalf("exit key = %d, want %d", got.Kind, KeyCtrlC)
	}
}

func TestPromptStashIsUnboundByDefaultLikeOpenCode(t *testing.T) {
	if got := DefaultKeymap().Binding(ActionStash); got != "none" {
		t.Fatalf("default stash binding = %q, want none", got)
	}
}

func TestPromptClearUsesCtrlCByDefault(t *testing.T) {
	keymap := DefaultKeymap()
	if keymap.Binding(ActionClear) != "ctrl+c" || !keymap.Matches(ActionClear, Key{Kind: KeyCtrlC}) {
		t.Fatalf("clear binding does not match ctrl+c: %q", keymap.Binding(ActionClear))
	}
}

func TestPromptClearCanBeRemappedWithoutChangingExit(t *testing.T) {
	keymap := DefaultKeymap()
	keymap.bindings[ActionClear] = "ctrl+l"
	if !keymap.Matches(ActionClear, Key{Kind: KeyCtrlL}) {
		t.Fatal("configured clear key did not match")
	}
	if keymap.Matches(ActionClear, Key{Kind: KeyCtrlC}) {
		t.Fatal("old clear key still matched after remap")
	}
	if !keymap.Matches(ActionExit, Key{Kind: KeyCtrlC}) {
		t.Fatal("exit binding changed when clear was remapped")
	}
}

func TestMessageNavigationBindingsAreConfigurable(t *testing.T) {
	keymap := DefaultKeymap()
	// OpenCode binds messages_first to ctrl+g,home and messages_page_up/down to
	// pageup/pagedown. Both ctrl+g and home must normalize to the first-message action.
	if keymap.Binding(ActionFirst) != "ctrl+g,home" || keymap.Binding(ActionPageUp) != "pageup" || keymap.Binding(ActionPageDown) != "pagedown" {
		t.Fatalf("message bindings = %#v", keymap.bindings)
	}
	if keymap.Normalize(Key{Kind: KeyCtrlG}).Kind != KeyMessageFirst {
		t.Fatal("ctrl+g did not select first-message action")
	}
	if keymap.Normalize(Key{Kind: KeyHome}).Kind != KeyMessageFirst {
		t.Fatal("home did not select first-message action")
	}
}

func TestInputEditBindingsMatchOpenCode(t *testing.T) {
	keymap := DefaultKeymap()
	// OpenCode: ctrl+a = line home, super+a = select all, ctrl+e = line end.
	if keymap.Binding(ActionLineHome) != "ctrl+a" {
		t.Fatalf("line home = %q, want ctrl+a", keymap.Binding(ActionLineHome))
	}
	if keymap.Binding(ActionLineEnd) != "ctrl+e" {
		t.Fatalf("line end = %q, want ctrl+e", keymap.Binding(ActionLineEnd))
	}
	if keymap.Binding(ActionSelectAll) != "super+a" {
		t.Fatalf("select all = %q, want super+a", keymap.Binding(ActionSelectAll))
	}
	if keymap.Normalize(Key{Kind: KeyLineHome}).Kind != KeyLineHome {
		t.Fatal("ctrl+a did not normalize to line home")
	}
	if keymap.Normalize(Key{Kind: KeyLineEnd}).Kind != KeyLineEnd {
		t.Fatal("ctrl+e did not normalize to line end")
	}
	if keymap.Normalize(Key{Kind: KeySelectAll}).Kind != KeySelectAll {
		t.Fatal("super+a did not normalize to select all")
	}
	// Delete-to-line-end/start and undo.
	if keymap.Binding(ActionDeleteToLineEnd) != "ctrl+k" || keymap.Binding(ActionDeleteToLineStart) != "ctrl+u" {
		t.Fatalf("delete-to-line bindings = %q/%q", keymap.Binding(ActionDeleteToLineEnd), keymap.Binding(ActionDeleteToLineStart))
	}
	if keymap.Normalize(Key{Kind: KeyDeleteToLineEnd}).Kind != KeyDeleteToLineEnd {
		t.Fatal("ctrl+k did not normalize to delete-to-line-end")
	}
	if keymap.Normalize(Key{Kind: KeyDeleteToLineStart}).Kind != KeyDeleteToLineStart {
		t.Fatal("ctrl+u did not normalize to delete-to-line-start")
	}
	if keymap.Normalize(Key{Kind: KeyUndo}).Kind != KeyUndo {
		t.Fatal("ctrl+- did not normalize to undo")
	}
}
