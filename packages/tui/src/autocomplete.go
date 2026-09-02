package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AutocompleteKind string

const (
	AutocompleteNone    AutocompleteKind = ""
	AutocompleteFile    AutocompleteKind = "@"
	AutocompleteCommand AutocompleteKind = "/"
)

type Autocomplete struct {
	Kind     AutocompleteKind
	Query    string
	Start    int
	Items    []PickerItem
	Index    int
	Visible  bool
	Commands []PickerItem
}

func NewAutocomplete() *Autocomplete { return &Autocomplete{} }

func (a *Autocomplete) Refresh(value string, cursor int, root string) {
	runes := []rune(value)
	if cursor < 0 || cursor > len(runes) {
		cursor = len(runes)
	}
	lineStart := cursor
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	tokenStart := cursor
	for tokenStart > lineStart && !isAutocompleteBoundary(runes[tokenStart-1]) {
		tokenStart--
	}
	if tokenStart >= cursor {
		a.Close()
		return
	}
	trigger := AutocompleteNone
	if runes[tokenStart] == '@' {
		trigger = AutocompleteFile
	}
	if runes[tokenStart] == '/' && tokenStart == lineStart {
		trigger = AutocompleteCommand
	}
	if trigger == AutocompleteNone {
		a.Close()
		return
	}
	query := string(runes[tokenStart+1 : cursor])
	items := a.options(trigger, query, root)
	a.Kind, a.Query, a.Start, a.Items, a.Index, a.Visible = trigger, query, tokenStart, items, 0, len(items) > 0
}

func (a *Autocomplete) options(kind AutocompleteKind, query, root string) []PickerItem {
	if kind == AutocompleteCommand {
		commands := append([]PickerItem(nil), a.Commands...)
		if len(commands) == 0 {
			commands = []PickerItem{{ID: "/help", Label: "/help", Description: "Show help"}, {ID: "/status", Label: "/status", Description: "Show status"}, {ID: "/usage", Label: "/usage", Description: "Show provider usage"}, {ID: "/models", Label: "/models", Description: "Choose a model"}, {ID: "/agents", Label: "/agents", Description: "Choose an agent"}, {ID: "/sessions", Label: "/sessions", Description: "Switch session"}, {ID: "/resume", Label: "/resume", Description: "Resume a session"}, {ID: "/continue", Label: "/continue", Description: "Continue a session"}, {ID: "/new", Label: "/new", Description: "Create a session"}, {ID: "/clear", Label: "/clear", Description: "Create a session"}, {ID: "/fork", Label: "/fork", Description: "Fork the current session"}, {ID: "/rename", Label: "/rename", Description: "Rename the current session"}, {ID: "/export", Label: "/export", Description: "Export the current session"}, {ID: "/history", Label: "/history", Description: "Show session history"}, {ID: "/skills", Label: "/skills", Description: "List skills"}, {ID: "/mcps", Label: "/mcps", Description: "Show MCP integrations"}, {ID: "/lsp", Label: "/lsp", Description: "Show LSP integrations"}, {ID: "/plugins", Label: "/plugins", Description: "Show plugin integrations"}, {ID: "/editor", Label: "/editor", Description: "Open external editor"}, {ID: "/exit", Label: "/exit", Description: "Exit"}, {ID: "/quit", Label: "/quit", Description: "Exit"}, {ID: "/q", Label: "/q", Description: "Exit"}}
		}
		return filterAutocomplete(commands, query)
	}
	if root == "" {
		return nil
	}
	var items []PickerItem
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		items = append(items, PickerItem{ID: relative, Label: relative, Description: "file"})
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return filterAutocomplete(items, query)
}

func filterAutocomplete(items []PickerItem, query string) []PickerItem {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		if len(items) > 20 {
			return items[:20]
		}
		return items
	}
	result := make([]PickerItem, 0, len(items))
	for _, item := range items {
		value := strings.ToLower(strings.ReplaceAll(item.ID, "\\", "/") + " " + item.Description)
		if strings.Contains(value, strings.ReplaceAll(query, "\\", "/")) {
			result = append(result, item)
		}
	}
	if len(result) > 20 {
		return result[:20]
	}
	return result
}

func isAutocompleteBoundary(value rune) bool { return value == ' ' || value == '\n' || value == '\t' }
func (a *Autocomplete) Close() {
	a.Kind, a.Query, a.Start, a.Items, a.Index, a.Visible = AutocompleteNone, "", 0, nil, 0, false
}
func (a *Autocomplete) Move(delta int) {
	if len(a.Items) == 0 {
		return
	}
	a.Index = (a.Index + delta) % len(a.Items)
	if a.Index < 0 {
		a.Index = len(a.Items) - 1
	}
}
func (a *Autocomplete) Selected() (PickerItem, bool) {
	if !a.Visible || a.Index < 0 || a.Index >= len(a.Items) {
		return PickerItem{}, false
	}
	return a.Items[a.Index], true
}

func (a *Autocomplete) Accept(editor *Editor) bool {
	item, ok := a.Selected()
	if !ok {
		return false
	}
	value := []rune(editor.String())
	cursor := editor.Cursor()
	start := a.Start
	if start < 0 || start >= len(value) || cursor < start+1 {
		a.Close()
		return false
	}
	replacement := item.ID
	if a.Kind == AutocompleteFile {
		replacement = "@" + replacement
	}
	if a.Kind == AutocompleteCommand {
		replacement = item.ID
	}
	next := append([]rune{}, value[:start]...)
	next = append(next, []rune(replacement)...)
	next = append(next, value[cursor:]...)
	editor.value, editor.cursor = next, start+len([]rune(replacement))
	if a.Kind == AutocompleteFile {
		editor.Insert(" ")
	}
	a.Close()
	return true
}
