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
	Agents   []PickerItem
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
			commands = []PickerItem{
				{ID: "/models", Label: "/models", Description: "Select provider model"},
				{ID: "/variants", Label: "/variants", Description: "Select model variant"},
				{ID: "/agents", Label: "/agents", Description: "Select agent (build, plan)"},
				{ID: "/sessions", Label: "/sessions", Description: "List and switch sessions"},
				{ID: "/resume", Label: "/resume", Description: "Resume a session"},
				{ID: "/continue", Label: "/continue", Description: "Continue a session"},
				{ID: "/new", Label: "/new", Description: "Start a new session"},
				{ID: "/fork", Label: "/fork", Description: "Fork current session"},
				{ID: "/rename", Label: "/rename", Description: "Rename current session"},
				{ID: "/diff", Label: "/diff", Description: "View git working tree diff"},
				{ID: "/git", Label: "/git", Description: "View git status and branch"},
				{ID: "/mcps", Label: "/mcps", Description: "Show MCP server status and tools"},
				{ID: "/lsp", Label: "/lsp", Description: "Show LSP server status"},
				{ID: "/plugins", Label: "/plugins", Description: "Show plugin status"},
				{ID: "/skills", Label: "/skills", Description: "List available skills"},
				{ID: "/status", Label: "/status", Description: "Show current session and model info"},
				{ID: "/usage", Label: "/usage", Description: "Show token usage statistics"},
				{ID: "/stash", Label: "/stash", Description: "Save or view stashed prompts"},
				{ID: "/editor", Label: "/editor", Description: "Open external editor"},
				{ID: "/clear", Label: "/clear", Description: "Clear conversation or prompt"},
				{ID: "/help", Label: "/help", Description: "Show available commands"},
				{ID: "/exit", Label: "/exit", Description: "Exit application"},
			}
		}
		return filterAutocomplete(commands, query)
	}
	if kind != AutocompleteFile {
		return nil
	}
	var items []PickerItem
	for _, agent := range a.Agents {
		items = append(items, agent)
	}
	if root != "" {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil || rel == "." {
				return nil
			}
			if info.IsDir() {
				if info.Name() == ".git" || info.Name() == "node_modules" || info.Name() == ".yteam" {
					return filepath.SkipDir
				}
				return nil
			}
			itemPath := filepath.ToSlash(rel)
			items = append(items, PickerItem{ID: "@" + itemPath, Label: "@" + itemPath, Description: rel})
			return nil
		})
	}
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
		replacement = "@" + strings.TrimPrefix(item.ID, "@")
		if filepath.Separator == '\\' {
			replacement = "@" + filepath.FromSlash(strings.TrimPrefix(replacement, "@"))
		}
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
