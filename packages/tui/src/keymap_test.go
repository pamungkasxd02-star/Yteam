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
