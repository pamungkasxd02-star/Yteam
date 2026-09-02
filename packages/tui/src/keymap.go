package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Action string

const (
	ActionSubmit             Action = "input.submit"
	ActionNewline            Action = "input.newline"
	ActionBackspace          Action = "input.backspace"
	ActionDelete             Action = "input.delete"
	ActionWordBackward       Action = "input.word.backward"
	ActionWordForward        Action = "input.word.forward"
	ActionDeleteWordBackward Action = "input.delete.word.backward"
	ActionDeleteWordForward  Action = "input.delete.word.forward"
	ActionHistoryPrevious    Action = "prompt.history.previous"
	ActionHistoryNext        Action = "prompt.history.next"
	ActionPageUp             Action = "messages.page_up"
	ActionPageDown           Action = "messages.page_down"
	ActionExit               Action = "app.exit"
	ActionEditor             Action = "prompt.editor"
	ActionStash              Action = "prompt.stash"
	ActionClear              Action = "prompt.clear"
)

type KeymapConfig struct {
	Keybinds map[string]string `json:"keybinds,omitempty"`
}

type Keymap struct {
	bindings map[Action]string
}

func DefaultKeymap() *Keymap {
	return &Keymap{bindings: map[Action]string{
		ActionSubmit:             "return",
		ActionNewline:            "ctrl+j",
		ActionBackspace:          "backspace",
		ActionDelete:             "delete",
		ActionWordBackward:       "alt+left",
		ActionWordForward:        "alt+right",
		ActionDeleteWordBackward: "ctrl+w",
		ActionDeleteWordForward:  "ctrl+delete",
		ActionHistoryPrevious:    "up",
		ActionHistoryNext:        "down",
		ActionPageUp:             "pageup",
		ActionPageDown:           "pagedown",
		ActionExit:               "ctrl+c",
		ActionEditor:             "ctrl+e",
		ActionStash:              "none",
		ActionClear:              "ctrl+c",
	}}
}

func LoadKeymap(home string) (*Keymap, error) {
	keymap := DefaultKeymap()
	path := strings.TrimSpace(os.Getenv("YTEAM_TUI_CONFIG"))
	if path == "" && home != "" {
		path = filepath.Join(home, "tui.json")
	}
	if path == "" {
		return keymap, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return keymap, nil
	}
	if err != nil {
		return nil, err
	}
	var config KeymapConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	for name, value := range config.Keybinds {
		action := Action(strings.TrimSpace(name))
		if _, exists := keymap.bindings[action]; !exists {
			continue
		}
		if normalized := normalizeKeyName(value); normalized != "" {
			keymap.bindings[action] = normalized
		}
	}
	return keymap, nil
}

func normalizeKeyName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	aliases := map[string]string{"enter": "return", "esc": "escape", "pgup": "pageup", "pgdown": "pagedown", "del": "delete"}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '+' || r == ',' || r == ' ' })
	for index, part := range parts {
		if replacement, ok := aliases[part]; ok {
			parts[index] = replacement
		}
	}
	return strings.Join(parts, "+")
}

func (k *Keymap) Binding(action Action) string {
	if k == nil {
		return ""
	}
	return k.bindings[action]
}

func (k *Keymap) Matches(action Action, key Key) bool {
	if k == nil {
		return false
	}
	return keyBindingMatches(key, k.Binding(action))
}

func (k *Keymap) Normalize(key Key) Key {
	if k == nil {
		return key
	}
	items := []struct {
		action Action
		kind   KeyKind
	}{
		{ActionSubmit, KeyEnter}, {ActionNewline, KeyCtrlJ},
		{ActionBackspace, KeyBackspace}, {ActionDelete, KeyDelete},
		{ActionWordBackward, KeyWordLeft}, {ActionWordForward, KeyWordRight},
		{ActionDeleteWordBackward, KeyDeleteWordBackward}, {ActionDeleteWordForward, KeyDeleteWordForward},
		{ActionHistoryPrevious, KeyUp}, {ActionHistoryNext, KeyDown},
		{ActionPageUp, KeyPageUp}, {ActionPageDown, KeyPageDown},
		{ActionEditor, KeyOpenEditor},
		{ActionStash, KeyStash},
		{ActionClear, KeyClear},
		{ActionExit, KeyCtrlC},
	}
	for _, item := range items {
		if keyBindingMatches(key, k.Binding(item.action)) {
			key.Kind = item.kind
			key.Text = ""
			return key
		}
	}
	return key
}

func keyBindingMatches(key Key, binding string) bool {
	switch normalizeKeyName(binding) {
	case "return":
		return key.Kind == KeyEnter
	case "ctrl+j":
		return key.Kind == KeyCtrlJ
	case "ctrl+n":
		return key.Kind == KeyCtrlN
	case "backspace":
		return key.Kind == KeyBackspace
	case "delete":
		return key.Kind == KeyDelete
	case "alt+left", "ctrl+left":
		return key.Kind == KeyWordLeft
	case "alt+right", "ctrl+right":
		return key.Kind == KeyWordRight
	case "ctrl+w":
		return key.Kind == KeyDeleteWordBackward
	case "ctrl+delete":
		return key.Kind == KeyDeleteWordForward
	case "up":
		return key.Kind == KeyUp
	case "down":
		return key.Kind == KeyDown
	case "pageup":
		return key.Kind == KeyPageUp
	case "pagedown":
		return key.Kind == KeyPageDown
	case "ctrl+e":
		return key.Kind == KeyCtrlE
	case "ctrl+s":
		return key.Kind == KeyStash
	case "ctrl+c":
		return key.Kind == KeyCtrlC
	case "ctrl+l":
		return key.Kind == KeyCtrlL
	case "none":
		return false
	case "ctrl+q":
		return key.Kind == KeyCtrlQ
	}
	return false
}
