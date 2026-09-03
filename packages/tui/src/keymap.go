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
	ActionLineUp             Action = "messages.line_up"
	ActionLineDown           Action = "messages.line_down"
	ActionHalfPageUp         Action = "messages.half_page_up"
	ActionHalfPageDown       Action = "messages.half_page_down"
	ActionFirst              Action = "messages.first"
	ActionLast               Action = "messages.last"
	ActionSelectAll          Action = "input.select.all"
	ActionSelectLeft         Action = "input.select.left"
	ActionSelectRight        Action = "input.select.right"
	ActionSelectUp           Action = "input.select.up"
	ActionSelectDown         Action = "input.select.down"
	ActionExit               Action = "app.exit"
	ActionEditor             Action = "prompt.editor"
	ActionStash              Action = "prompt.stash"
	ActionClear              Action = "prompt.clear"
	ActionPaste              Action = "input.paste"
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
		ActionLineUp:             "none",
		ActionLineDown:           "none",
		ActionHalfPageUp:         "none",
		ActionHalfPageDown:       "none",
		ActionFirst:              "ctrl+g",
		ActionLast:               "none",
		ActionSelectAll:          "ctrl+a",
		ActionSelectLeft:         "shift+left",
		ActionSelectRight:        "shift+right",
		ActionSelectUp:           "shift+up",
		ActionSelectDown:         "shift+down",
		ActionExit:               "ctrl+c",
		ActionEditor:             "ctrl+e",
		ActionStash:              "none",
		ActionClear:              "ctrl+c",
		ActionPaste:              "ctrl+v",
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
		{ActionLineUp, KeyMessageLineUp}, {ActionLineDown, KeyMessageLineDown},
		{ActionHalfPageUp, KeyMessageHalfPageUp}, {ActionHalfPageDown, KeyMessageHalfPageDown},
		{ActionFirst, KeyMessageFirst}, {ActionLast, KeyMessageLast},
		{ActionSelectAll, KeySelectAll}, {ActionSelectLeft, KeySelectLeft}, {ActionSelectRight, KeySelectRight},
		{ActionSelectUp, KeySelectUp}, {ActionSelectDown, KeySelectDown},
		{ActionEditor, KeyOpenEditor},
		{ActionStash, KeyStash},
		{ActionClear, KeyClear},
		{ActionPaste, KeyClipboardPaste},
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
	case "ctrl+g":
		return key.Kind == KeyCtrlG
	case "home":
		return key.Kind == KeyHome
	case "end":
		return key.Kind == KeyEnd
	case "ctrl+alt+y":
		return key.Kind == KeyMessageLineUp
	case "ctrl+alt+e":
		return key.Kind == KeyMessageLineDown
	case "ctrl+alt+u":
		return key.Kind == KeyMessageHalfPageUp
	case "ctrl+alt+d":
		return key.Kind == KeyMessageHalfPageDown
	case "ctrl+a":
		return key.Kind == KeySelectAll
	case "shift+left":
		return key.Kind == KeySelectLeft
	case "shift+right":
		return key.Kind == KeySelectRight
	case "shift+up":
		return key.Kind == KeySelectUp
	case "shift+down":
		return key.Kind == KeySelectDown
	case "ctrl+e":
		return key.Kind == KeyCtrlE
	case "ctrl+s":
		return key.Kind == KeyStash
	case "ctrl+c":
		return key.Kind == KeyCtrlC
	case "ctrl+l":
		return key.Kind == KeyCtrlL
	case "ctrl+v":
		return key.Kind == KeyClipboardPaste
	case "none":
		return false
	case "ctrl+q":
		return key.Kind == KeyCtrlQ
	}
	return false
}
