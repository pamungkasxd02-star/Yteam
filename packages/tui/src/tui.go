package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	commandpkg "github.com/pamungkasxd02-star/Yteam/packages/command/src"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Route string

const (
	RouteHome    Route = "home"
	RouteSession Route = "session"
)

type UI struct {
	app           *runtime.Runtime
	in            io.Reader
	out           io.Writer
	mu            sync.Mutex
	route         Route
	input         string
	palette       bool
	paletteQuery  string
	selected      int
	transcript    []session.Message
	picker        *Picker
	pickerKind    string
	editor        *Editor
	reducer       *TranscriptReducer
	redraw        chan struct{}
	terminal      *os.File
	viewport      *Viewport
	autocomplete  *Autocomplete
	keymap        *Keymap
	promptBusy    bool
	promptDone    chan error
	promptHistory *PromptHistory
	promptParts   []schema.MessagePart
	promptStash   *PromptStash
	questionID    string
	questionAt    int
	questionSet   map[int]map[int]bool
	questionText  map[int]string
	questionMode  bool
	questionDone  bool
	exitRequested bool
}

func New(app *runtime.Runtime, in io.Reader, out io.Writer) *UI {
	reducer := NewTranscriptReducer()
	current := app.CurrentSession()
	reducer.Hydrate(current.Messages)
	autocomplete := NewAutocomplete()
	for _, item := range app.CommandList() {
		autocomplete.Commands = append(autocomplete.Commands, PickerItem{ID: "/" + item.Name, Label: "/" + item.Name, Description: item.Description})
	}
	for _, item := range commandpkg.AliasItems() {
		autocomplete.Commands = append(autocomplete.Commands, PickerItem{ID: item.ID, Label: item.Label, Description: item.Description})
	}
	for _, item := range []PickerItem{{ID: "/help", Label: "/help", Description: "Show help"}, {ID: "/status", Label: "/status", Description: "Show status"}, {ID: "/usage", Label: "/usage", Description: "Show provider usage"}, {ID: "/models", Label: "/models", Description: "Choose a model"}, {ID: "/variants", Label: "/variants", Description: "Choose a model variant"}, {ID: "/agents", Label: "/agents", Description: "Choose an agent"}, {ID: "/sessions", Label: "/sessions", Description: "Switch session"}, {ID: "/resume", Label: "/resume", Description: "Resume a session"}, {ID: "/continue", Label: "/continue", Description: "Continue a session"}, {ID: "/new", Label: "/new", Description: "Create a session"}, {ID: "/clear", Label: "/clear", Description: "Create a session"}, {ID: "/fork", Label: "/fork", Description: "Fork the current session"}, {ID: "/rename", Label: "/rename", Description: "Rename the current session"}, {ID: "/export", Label: "/export", Description: "Export the current session"}, {ID: "/history", Label: "/history", Description: "Show session history"}, {ID: "/stash", Label: "/stash", Description: "Stash or restore prompts"}, {ID: "/skills", Label: "/skills", Description: "List skills"}, {ID: "/mcps", Label: "/mcps", Description: "Show MCP integrations"}, {ID: "/lsp", Label: "/lsp", Description: "Show LSP integrations"}, {ID: "/plugins", Label: "/plugins", Description: "Show plugin integrations"}, {ID: "/editor", Label: "/editor", Description: "Open external editor"}, {ID: "/exit", Label: "/exit", Description: "Exit"}, {ID: "/quit", Label: "/quit", Description: "Exit"}, {ID: "/q", Label: "/q", Description: "Exit"}} {
		found := false
		for _, existing := range autocomplete.Commands {
			if existing.ID == item.ID {
				found = true
				break
			}
		}
		if !found {
			autocomplete.Commands = append(autocomplete.Commands, item)
		}
	}
	history, _ := OpenPromptHistory(app.Config.Home)
	stash, _ := OpenPromptStash(app.Config.Home)
	keymap, _ := LoadKeymap(app.Config.Home)
	return &UI{app: app, in: in, out: out, route: RouteHome, transcript: current.Messages, editor: NewEditor(), reducer: reducer, redraw: make(chan struct{}, 1), autocomplete: autocomplete, keymap: keymap, questionSet: map[int]map[int]bool{}, questionText: map[int]string{}, promptDone: make(chan error, 1), promptHistory: history, promptStash: stash, viewport: NewViewport(80, 18)}
}

func (ui *UI) Run(ctx context.Context) error {
	ui.startEventWatcher(ctx)
	if file, ok := ui.in.(*os.File); ok && IsTerminal(file) {
		return ui.runRaw(ctx, file)
	}
	ui.draw()
	scanner := bufio.NewScanner(ui.in)
	for {
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := normalizeLine(scanner.Text())
		if line == "\x03" || strings.TrimSpace(line) == "/exit" || strings.TrimSpace(line) == "/quit" || strings.TrimSpace(line) == "/q" {
			return nil
		}
		if ui.picker != nil {
			if err := ui.handlePickerLine(ctx, line); err != nil {
				fmt.Fprintln(ui.out, "error:", err)
			}
			if ui.exitRequested {
				return nil
			}
			ui.draw()
			continue
		}
		if strings.HasPrefix(line, "/") {
			if handled, err := ui.command(ctx, line); handled {
				if err != nil {
					fmt.Fprintln(ui.out, "error:", err)
				}
				ui.draw()
				continue
			}
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		ui.route = RouteSession
		if err := ui.app.Prompt(ctx, line, ui.out); err != nil {
			fmt.Fprintln(ui.out, "error:", err)
		}
		ui.transcript = ui.app.CurrentSession().Messages
		ui.draw()
	}
}

func (ui *UI) startEventWatcher(ctx context.Context) {
	journal := ui.app.EventJournal()
	if journal == nil {
		return
	}
	updates := journal.Subscribe(ctx)
	go func() {
		for event := range updates {
			if event.Aggregate != "" && event.Aggregate != ui.app.CurrentSession().ID {
				continue
			}
			ui.mu.Lock()
			ui.reducer.Apply(event)
			ui.mu.Unlock()
			select {
			case ui.redraw <- struct{}{}:
			default:
			}
		}
	}()
}

func (ui *UI) runRaw(ctx context.Context, file *os.File) error {
	ui.terminal = file
	width, height := terminalSize(file)
	ui.viewport.SetSize(width, height-6)
	restore, err := enableRaw(file)
	if err != nil {
		return err
	}
	terminal := newTerminalModes(file, ui.out, restore)
	if err := terminal.EnablePaste(); err != nil {
		_ = terminal.Close()
		return err
	}
	defer terminal.Close()
	resizeEvents, stopResize := watchTerminalResize()
	defer stopResize()
	ui.draw()
	keys := NewKeyReader(file)
	type keyResult struct {
		key Key
		err error
	}
	keyCh := make(chan keyResult, 1)
	go func() {
		for {
			key, err := keys.ReadKey()
			keyCh <- keyResult{key: key, err: err}
			if err != nil {
				return
			}
		}
	}()
	for {
		var key Key
		select {
		case result := <-keyCh:
			if result.err != nil {
				return result.err
			}
			key = result.key
		case <-ui.redraw:
			ui.draw()
			continue
		case err := <-ui.promptDone:
			ui.promptBusy = false
			if err != nil {
				fmt.Fprintln(ui.out, "error:", err)
			}
			ui.transcript = ui.app.CurrentSession().Messages
			ui.reducer.Hydrate(ui.transcript)
			ui.draw()
			continue
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-resizeEvents:
			if ok {
				width, height := terminalSize(file)
				ui.viewport.SetSize(width, height-6)
				ui.draw()
			}
			continue
		}
		rawKey := key
		key = ui.keymap.Normalize(key)
		if ui.picker != nil {
			if err := ui.handlePickerKey(ctx, key); err != nil {
				fmt.Fprintln(ui.out, "error:", err)
			}
			if ui.exitRequested {
				return nil
			}
			ui.draw()
			continue
		}
		if ui.handleQuestionKey(ctx, key) {
			ui.draw()
			continue
		}
		if key.Kind == KeyClear {
			if ui.promptHasContent() {
				if err := ui.clearPrompt(); err != nil {
					fmt.Fprintln(ui.out, "clear error:", err)
				}
				ui.draw()
				continue
			}
			if ui.keymap.Matches(ActionExit, rawKey) {
				ui.app.InterruptSession(ui.app.CurrentSession().ID)
				return nil
			}
			continue
		}
		if ui.keymap.Matches(ActionExit, rawKey) && rawKey.Kind == KeyCtrlC {
			ui.app.InterruptSession(ui.app.CurrentSession().ID)
			return nil
		}
		switch key.Kind {
		case KeyStash:
			if err := ui.stashCurrentPrompt(); err != nil {
				fmt.Fprintln(ui.out, "stash error:", err)
			}
		case KeyOpenEditor:
			value, editorErr := ui.openEditor(ctx, terminal, ui.editor.String())
			if editorErr != nil {
				fmt.Fprintln(ui.out, "editor error:", editorErr)
			} else if value != "" {
				ui.editor.Set(normalizePromptContent(value))
			}
		case KeyCtrlN:
			ui.insertEditorText("\n")
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyText:
			if ui.handlePermissionKey(key) {
				continue
			}
			ui.insertEditorText(key.Text)
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyPaste:
			ui.insertPaste(key.Text)
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyCtrlJ:
			ui.editor.Newline()
			ui.promptParts = nil
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyTab:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.acceptAutocomplete()
				ui.refreshAutocomplete()
			}
		case KeyEnter:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.acceptAutocomplete()
				ui.refreshAutocomplete()
				ui.draw()
				continue
			}
			content := normalizePromptContent(ui.editor.String())
			text := strings.TrimSpace(content)
			if text == "" {
				ui.draw()
				continue
			}
			if ui.promptBusy {
				fmt.Fprintln(ui.out, "\nA run is still active. Press Ctrl+C to interrupt it.")
				ui.draw()
				continue
			}
			if text == "/editor" {
				value, editorErr := ui.openEditor(ctx, terminal, "")
				if editorErr != nil {
					fmt.Fprintln(ui.out, "editor error:", editorErr)
				} else if value != "" {
					ui.editor.Set(normalizePromptContent(value))
				}
				ui.draw()
				continue
			}
			parts := clonePromptParts(ui.promptParts)
			expanded := expandTrackedPastedText(content, parts)
			ui.editor.AddHistory(content)
			if ui.promptHistory != nil {
				if err := ui.promptHistory.Append(PromptEntry{Input: content, Mode: "normal", Parts: parts}); err != nil {
					fmt.Fprintln(ui.out, "history error:", err)
				}
			}
			ui.editor.Reset()
			ui.promptParts = nil
			if strings.HasPrefix(text, "/") {
				if ui.isPromptCommand(text) {
					ui.route = RouteSession
					ui.promptBusy = true
					go func() {
						_, err := ui.app.Command(ctx, text, ui)
						ui.promptDone <- err
					}()
					ui.draw()
					continue
				}
				if text == "/exit" || text == "/quit" || text == "/q" {
					ui.app.InterruptSession(ui.app.CurrentSession().ID)
					return nil
				}
				handled, commandErr := ui.command(ctx, text)
				if handled {
					if commandErr != nil {
						fmt.Fprintln(ui.out, "error:", commandErr)
					}
					ui.transcript = ui.app.CurrentSession().Messages
					ui.draw()
					continue
				}
			}
			ui.route = RouteSession
			ui.promptBusy = true
			go func(prompt string, promptParts []schema.MessagePart) {
				ui.promptDone <- ui.app.PromptWithParts(ctx, prompt, promptParts, ui)
			}(expanded, parts)
		case KeyBackspace:
			start := previousClusterStart(ui.editor.value, ui.editor.cursor)
			ui.rebaseEditorEdit(start, ui.editor.cursor, "")
			ui.editor.Backspace()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyDelete:
			end := nextClusterEnd(ui.editor.value, ui.editor.cursor)
			ui.rebaseEditorEdit(ui.editor.cursor, end, "")
			ui.editor.Delete()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyLeft:
			ui.editor.Left()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyRight:
			ui.editor.Right()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyWordLeft:
			ui.editor.WordLeft()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyWordRight:
			ui.editor.WordRight()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyDeleteWordBackward:
			before := ui.editor.cursor
			ui.editor.WordLeft()
			ui.rebaseEditorEdit(ui.editor.cursor, before, "")
			ui.editor.value = append(ui.editor.value[:ui.editor.cursor], ui.editor.value[before:]...)
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyDeleteWordForward:
			before := ui.editor.cursor
			ui.editor.WordRight()
			ui.rebaseEditorEdit(before, ui.editor.cursor, "")
			ui.editor.value = append(ui.editor.value[:before], ui.editor.value[ui.editor.cursor:]...)
			ui.editor.cursor = before
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyHome:
			ui.editor.Home()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyEnd:
			ui.editor.End()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyUp:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.autocomplete.Move(-1)
				ui.draw()
				continue
			}
			atBufferStart := ui.editor.Cursor() == 0
			atLineStart := ui.editor.Cursor() == lineStart(ui.editor.value, ui.editor.cursor)
			if ui.promptHistory != nil && atBufferStart && atLineStart {
				if value, ok := ui.promptHistory.Move(-1, ui.editor.String()); ok {
					ui.editor.Set(value.Input)
					ui.promptParts = clonePromptParts(value.Parts)
				}
			} else if atLineStart && atBufferStart {
				ui.editor.HistoryUp()
			} else {
				ui.editor.Up()
			}
		case KeyDown:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.autocomplete.Move(1)
				ui.draw()
				continue
			}
			lastLine := lineEnd(ui.editor.value, ui.editor.cursor) == len(ui.editor.value)
			if ui.promptHistory != nil && lastLine {
				if value, ok := ui.promptHistory.Move(1, ui.editor.String()); ok {
					ui.editor.Set(value.Input)
					ui.promptParts = clonePromptParts(value.Parts)
				}
			} else if lastLine {
				ui.editor.HistoryDown()
			} else {
				ui.editor.Down()
			}
		case KeyPageUp, KeyPageDown:
			if key.Kind == KeyPageUp {
				ui.viewport.Page(-1)
			} else {
				ui.viewport.Page(1)
			}
		case KeyCtrlP:
			if ui.promptHistory != nil {
				if value, ok := ui.promptHistory.Move(-1, ui.editor.String()); ok {
					ui.editor.Set(value.Input)
					ui.promptParts = clonePromptParts(value.Parts)
				}
			} else {
				ui.editor.HistoryUp()
			}
		case KeyEscape:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.autocomplete.Close()
			} else {
				ui.clearPrompt()
			}
		}
		ui.draw()
	}
}

func (ui *UI) refreshAutocomplete() {
	if ui.autocomplete == nil {
		return
	}
	ui.autocomplete.Refresh(ui.editor.String(), ui.editor.Cursor(), ui.app.Root)
}

func (ui *UI) insertPaste(value string) {
	normalized := normalizePasteText(value)
	virtual, summarized := pastedVirtualText(normalized)
	if !summarized {
		ui.insertEditorText(normalized)
		ui.resetPromptHistoryNavigation()
		ui.refreshAutocomplete()
		return
	}
	start := ui.editor.Cursor()
	ui.rebaseEditorEdit(start, start, virtual+" ")
	ui.editor.Insert(virtual + " ")
	ui.promptParts = append(ui.promptParts, schema.MessagePart{
		Type:   "text",
		Text:   normalized,
		Source: &schema.PromptPartSource{Text: &schema.PromptTextSource{Start: start, End: start + len([]rune(virtual)), Value: virtual}},
	})
	ui.resetPromptHistoryNavigation()
	ui.refreshAutocomplete()
}

func (ui *UI) insertEditorText(value string) {
	start := ui.editor.Cursor()
	ui.rebaseEditorEdit(start, start, value)
	ui.editor.Insert(value)
}

func (ui *UI) rebaseEditorEdit(start, end int, inserted string) {
	ui.promptParts = rebasePromptParts(ui.promptParts, start, end, inserted)
}

func (ui *UI) acceptAutocomplete() {
	if ui.autocomplete == nil || !ui.autocomplete.Visible {
		return
	}
	item, ok := ui.autocomplete.Selected()
	if !ok {
		return
	}
	kind := ui.autocomplete.Kind
	start := ui.autocomplete.Start
	end := ui.editor.Cursor()
	replacement := item.ID
	if kind == AutocompleteFile {
		replacement = "@" + replacement
	}
	ui.rebaseEditorEdit(start, end, replacement)
	if !ui.autocomplete.Accept(ui.editor) {
		return
	}
	if kind != AutocompleteFile {
		return
	}
	virtual := "@" + item.ID
	ui.promptParts = append(ui.promptParts, schema.MessagePart{
		Type:     "file",
		Filename: item.ID,
		Source:   &schema.PromptPartSource{Type: "file", Path: item.ID, Text: &schema.PromptTextSource{Start: start, End: start + len([]rune(virtual)), Value: virtual}},
	})
}

func (ui *UI) popStash() error {
	if ui.promptStash == nil {
		return fmt.Errorf("prompt stash is not configured")
	}
	entry, ok, err := ui.promptStash.Pop()
	if err != nil || !ok {
		return err
	}
	ui.editor.Set(entry.Input)
	ui.promptParts = clonePromptParts(entry.Parts)
	return nil
}

func (ui *UI) openStashPicker() error {
	if ui.promptStash == nil {
		return fmt.Errorf("prompt stash is not configured")
	}
	entries := ui.promptStash.Entries()
	items := make([]PickerItem, 0, len(entries))
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		preview := strings.TrimSpace(strings.SplitN(entry.Input, "\n", 2)[0])
		if len([]rune(preview)) > 50 {
			preview = string([]rune(preview)[:50]) + "…"
		}
		items = append(items, PickerItem{ID: fmt.Sprintf("%d", index), Label: preview, Description: timeAgo(entry.Timestamp)})
	}
	ui.picker, ui.pickerKind = NewPicker("Stash", items), "stash"
	return nil
}

func (ui *UI) stashCurrentPrompt() error {
	if ui.promptStash == nil {
		return fmt.Errorf("prompt stash is not configured")
	}
	if ui.editor.Empty() && len(ui.promptParts) == 0 {
		return nil
	}
	if err := ui.promptStash.Push(StashEntry{Input: ui.editor.String(), Parts: clonePromptParts(ui.promptParts)}); err != nil {
		return err
	}
	ui.editor.Reset()
	ui.promptParts = nil
	fmt.Fprintln(ui.out, "Prompt stashed")
	return nil
}

func timeAgo(timestamp int64) string {
	if timestamp <= 0 {
		return "unknown time"
	}
	seconds := int64(time.Since(time.UnixMilli(timestamp)).Seconds())
	if seconds < 60 {
		return "just now"
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	return fmt.Sprintf("%dd ago", hours/24)
}

func (ui *UI) resetPromptHistoryNavigation() {
	if ui.promptHistory != nil {
		ui.promptHistory.ResetNavigation()
	}
}

func (ui *UI) promptHasContent() bool {
	return !ui.editor.Empty() || len(ui.promptParts) > 0
}

func (ui *UI) clearPrompt() error {
	content := ui.editor.String()
	if ui.promptHistory != nil && (len(strings.TrimSpace(content)) >= 20 || len(ui.promptParts) > 0) {
		if err := ui.promptHistory.Append(PromptEntry{Input: content, Mode: "normal", Parts: clonePromptParts(ui.promptParts)}); err != nil {
			return err
		}
	}
	ui.editor.Reset()
	ui.promptParts = nil
	if ui.autocomplete != nil {
		ui.autocomplete.Close()
	}
	ui.resetPromptHistoryNavigation()
	return nil
}

func (ui *UI) isPromptCommand(text string) bool {
	parts := strings.Fields(text)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return false
	}
	name := commandpkg.Canonical(parts[0])
	if name == "editor" {
		return true
	}
	_, ok := ui.app.Commands[name]
	return ok
}

func (ui *UI) openEditor(ctx context.Context, terminal *terminalModes, value string) (string, error) {
	if err := terminal.Suspend(); err != nil {
		return "", err
	}
	content, editorErr := openExternalEditor(ctx, value, ui.app.Root, ui.app.Config.Home, terminal.file, ui.out, ui.out)
	if resumeErr := terminal.Resume(); editorErr == nil && resumeErr != nil {
		return "", resumeErr
	} else if editorErr != nil && resumeErr != nil {
		return "", fmt.Errorf("%w; terminal resume failed: %v", editorErr, resumeErr)
	}
	return content, editorErr
}

func (ui *UI) handleQuestionKey(ctx context.Context, key Key) bool {
	items := ui.app.PendingQuestions(ui.app.CurrentSession().ID)
	if len(items) == 0 || len(items[0].Questions) == 0 {
		ui.resetQuestionState()
		return false
	}
	request := items[0]
	ui.prepareQuestionState(request)
	if ui.questionDone {
		if key.Kind == KeyEscape {
			ui.questionDone = false
			return true
		}
		if key.Kind == KeyEnter {
			answers, err := ui.questionAnswers(request)
			if err != nil {
				fmt.Fprintln(ui.out, "question error:", err)
				return true
			}
			if err := ui.app.ReplyQuestion(ctx, request.SessionID, request.ID, answers); err != nil {
				fmt.Fprintln(ui.out, "question error:", err)
			} else {
				ui.resetQuestionState()
			}
			return true
		}
		return key.Kind == KeyUp || key.Kind == KeyDown || key.Kind == KeyText
	}
	question := request.Questions[ui.questionAt]
	if key.Kind == KeyEscape {
		if ui.questionMode {
			ui.questionMode = false
			ui.questionText[ui.questionAt] = ""
			return true
		}
		if err := ui.app.RejectQuestion(ctx, request.SessionID, request.ID); err != nil {
			fmt.Fprintln(ui.out, "question error:", err)
		}
		ui.resetQuestionState()
		return true
	}
	if ui.questionMode {
		switch key.Kind {
		case KeyText:
			ui.questionText[ui.questionAt] += key.Text
		case KeyBackspace:
			ui.questionText[ui.questionAt] = dropLastRune(ui.questionText[ui.questionAt])
		case KeyEnter:
			if strings.TrimSpace(ui.questionText[ui.questionAt]) == "" {
				return true
			}
			ui.questionMode = false
			ui.advanceQuestion(request)
		case KeyCtrlJ:
			ui.questionText[ui.questionAt] += "\n"
		default:
			return true
		}
		return true
	}
	if key.Kind == KeyUp || key.Kind == KeyDown {
		if len(question.Options) == 0 {
			return true
		}
		if key.Kind == KeyUp {
			ui.questionAtOption(question, -1)
		} else {
			ui.questionAtOption(question, 1)
		}
		return true
	}
	if key.Kind == KeyText && strings.TrimSpace(key.Text) == "c" && question.Custom != nil && *question.Custom {
		ui.questionMode = true
		return true
	}
	if key.Kind == KeyText {
		choice := atoi(key.Text)
		if choice < 1 || choice > len(question.Options) {
			return true
		}
		if question.Multiple {
			if ui.questionSet[ui.questionAt] == nil {
				ui.questionSet[ui.questionAt] = map[int]bool{}
			}
			ui.questionSet[ui.questionAt][choice-1] = !ui.questionSet[ui.questionAt][choice-1]
			return true
		}
		ui.questionSet[ui.questionAt] = map[int]bool{choice - 1: true}
		ui.advanceQuestion(request)
		return true
	}
	if key.Kind == KeyEnter {
		ui.advanceQuestion(request)
		return true
	}
	return key.Kind == KeyTab || key.Kind == KeyBackspace || key.Kind == KeyDelete
}

func (ui *UI) prepareQuestionState(request schema.QuestionRequest) {
	if ui.questionID == request.ID {
		return
	}
	ui.questionID = request.ID
	ui.questionAt = 0
	ui.questionSet = map[int]map[int]bool{}
	ui.questionText = map[int]string{}
	ui.questionMode = false
	ui.questionDone = false
}

func (ui *UI) resetQuestionState() {
	ui.questionID = ""
	ui.questionAt = 0
	ui.questionSet = map[int]map[int]bool{}
	ui.questionText = map[int]string{}
	ui.questionMode = false
	ui.questionDone = false
}

func (ui *UI) advanceQuestion(request schema.QuestionRequest) {
	ui.questionAt++
	if ui.questionAt >= len(request.Questions) {
		ui.questionAt = len(request.Questions) - 1
		ui.questionDone = true
	}
}

func (ui *UI) questionAtOption(question schema.QuestionInfo, direction int) {
	if len(question.Options) == 0 {
		return
	}
	current := -1
	for index := range question.Options {
		if ui.questionSet[ui.questionAt] != nil && ui.questionSet[ui.questionAt][index] {
			current = index
			break
		}
	}
	if current < 0 {
		current = 0
	} else {
		current = (current + direction + len(question.Options)) % len(question.Options)
	}
	if !question.Multiple {
		ui.questionSet[ui.questionAt] = map[int]bool{current: true}
	}
}

func (ui *UI) questionAnswers(request schema.QuestionRequest) ([]schema.QuestionAnswer, error) {
	answers := make([]schema.QuestionAnswer, len(request.Questions))
	for index, question := range request.Questions {
		for optionIndex, option := range question.Options {
			if ui.questionSet[index] != nil && ui.questionSet[index][optionIndex] {
				answers[index] = append(answers[index], option.Label)
			}
		}
		if custom := strings.TrimSpace(ui.questionText[index]); custom != "" {
			answers[index] = append(answers[index], custom)
		}
		if len(answers[index]) == 0 {
			return nil, fmt.Errorf("question %d has no answer", index+1)
		}
	}
	return answers, nil
}

func (ui *UI) handlePermissionKey(key Key) bool {
	pending := ui.app.PendingPermissionsForSession(ui.app.CurrentSession().ID)
	if len(pending) == 0 || key.Kind != KeyText {
		return false
	}
	var reply permission.Reply
	switch strings.ToLower(key.Text) {
	case "y":
		reply = permission.Once
	case "a":
		reply = permission.Always
	case "n":
		reply = permission.Reject
	default:
		return false
	}
	if err := ui.app.ReplyPermission(pending[0].ID, reply); err != nil {
		fmt.Fprintln(ui.out, "permission error:", err)
	}
	return true
}

func (ui *UI) handlePickerKey(ctx context.Context, key Key) error {
	if ui.picker == nil {
		return nil
	}
	switch key.Kind {
	case KeyUp:
		ui.picker.Move(-1)
	case KeyDown:
		ui.picker.Move(1)
	case KeyPageUp:
		ui.picker.Page(-1)
	case KeyPageDown:
		ui.picker.Page(1)
	case KeyHome:
		ui.picker.Home()
	case KeyEnd:
		ui.picker.End()
	case KeyBackspace:
		ui.picker.SetQuery(dropLastRune(ui.picker.Query))
	case KeyText:
		ui.picker.SetQuery(ui.picker.Query + key.Text)
	case KeyEscape:
		ui.picker = nil
		ui.pickerKind = ""
	case KeyEnter:
		return ui.selectPicker(ctx)
	}
	return nil
}

func dropLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func editorWithCaret(editor *Editor) string {
	value := editor.String()
	if editor.cursor >= len([]rune(value)) {
		return value + "\x1b[7m \x1b[0m"
	}
	runes := []rune(value)
	return string(runes[:editor.cursor]) + "\x1b[7m" + string(runes[editor.cursor]) + "\x1b[0m" + string(runes[editor.cursor+1:])
}

func (ui *UI) command(ctx context.Context, line string) (bool, error) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return true, nil
	}
	switch parts[0] {
	case "/home":
		ui.route = RouteHome
		return true, nil
	case "/status", "/debug":
		ui.app.Status(ui.out)
		return true, nil
	case "/mcps":
		value := ui.app.MCP()
		data, _ := json.MarshalIndent(value, "", "  ")
		fmt.Fprintln(ui.out, string(data))
		return true, nil
	case "/usage":
		value := map[string]any{"total": ui.app.ProviderUsage(), "by_model": ui.app.ProviderUsageByModel()}
		data, _ := json.MarshalIndent(value, "", "  ")
		fmt.Fprintln(ui.out, string(data))
		return true, nil
	case "/lsp":
		data, _ := json.MarshalIndent(ui.app.LSP(), "", "  ")
		fmt.Fprintln(ui.out, string(data))
		return true, nil
	case "/plugins":
		data, _ := json.MarshalIndent(ui.app.Plugins(), "", "  ")
		fmt.Fprintln(ui.out, string(data))
		return true, nil
	case "/skills":
		items, err := ui.app.Skills()
		if err != nil {
			return true, err
		}
		for _, item := range items {
			fmt.Fprintf(ui.out, "%s — %s\n", item.Name, item.Description)
		}
		return true, nil
	case "/stash":
		if ui.promptStash == nil {
			return true, fmt.Errorf("prompt stash is not configured")
		}
		if len(parts) > 1 {
			switch parts[1] {
			case "pop":
				return true, ui.popStash()
			case "delete":
				if len(parts) < 3 {
					return true, fmt.Errorf("usage: /stash delete <index>")
				}
				return true, ui.promptStash.Remove(atoi(parts[2]) - 1)
			case "list":
				return true, ui.openStashPicker()
			default:
				return true, fmt.Errorf("usage: /stash [pop|list|delete <index>]")
			}
		}
		if ui.editor.Empty() {
			return true, ui.openStashPicker()
		}
		if err := ui.promptStash.Push(StashEntry{Input: ui.editor.String(), Parts: clonePromptParts(ui.promptParts)}); err != nil {
			return true, err
		}
		ui.editor.Reset()
		ui.promptParts = nil
		fmt.Fprintln(ui.out, "Prompt stashed")
		return true, nil
	case "/palette":
		items := make([]PickerItem, 0, len(ui.autocomplete.Commands))
		items = append(items, ui.autocomplete.Commands...)
		ui.picker, ui.pickerKind = NewPicker("Command palette", items), "command"
		return true, nil
	case "/models":
		models, err := ui.app.Models(ctx)
		if err != nil {
			return true, err
		}
		items := make([]PickerItem, 0, len(models))
		for _, model := range models {
			items = append(items, PickerItem{ID: model.ID, Label: model.ID})
		}
		ui.picker, ui.pickerKind = NewPicker("Select model", items), "model"
		return true, nil
	case "/variants", "/variant":
		if len(parts) > 1 {
			if err := ui.app.SetVariant(parts[1]); err != nil {
				return true, err
			}
			fmt.Fprintln(ui.out, "Active variant:", ui.app.VariantName())
			return true, nil
		}
		variants, err := ui.app.Variants(ctx)
		if err != nil {
			return true, err
		}
		items := make([]PickerItem, 0, len(variants))
		for _, variant := range variants {
			items = append(items, PickerItem{ID: variant, Label: variant})
		}
		ui.picker, ui.pickerKind = NewPicker("Select variant", items), "variant"
		return true, nil
	case "/agent", "/agents":
		if len(parts) < 2 {
			ui.picker, ui.pickerKind = NewPicker("Select agent", []PickerItem{{ID: "build", Label: "build", Description: "Implement changes and run tools"}, {ID: "plan", Label: "plan", Description: "Inspect the project and propose a plan"}}), "agent"
			return true, nil
		}
		if err := ui.app.SetAgent(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(ui.out, "Active agent:", ui.app.AgentName())
		return true, nil
	case "/model":
		if len(parts) < 2 {
			fmt.Fprintln(ui.out, "Active model:", ui.app.ModelName())
			return true, nil
		}
		if err := ui.app.SetModel(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(ui.out, "Active model:", ui.app.ModelName())
		return true, nil
	case "/sessions", "/resume", "/continue":
		items, err := ui.app.ListSessions()
		if err != nil {
			return true, err
		}
		options := make([]PickerItem, 0, len(items))
		for _, item := range items {
			options = append(options, PickerItem{ID: item.ID, Label: item.Title, Description: item.Directory})
		}
		ui.picker, ui.pickerKind = NewPicker("Select session", options), "session"
		return true, nil
	case "/new", "/clear":
		next, err := ui.app.NewSession()
		if err != nil {
			return true, err
		}
		ui.route = RouteHome
		ui.transcript = nil
		ui.reducer.Hydrate(nil)
		fmt.Fprintln(ui.out, "New session:", next.ID)
		return true, nil
	case "/fork":
		next, err := ui.app.ForkSession()
		if err != nil {
			return true, err
		}
		ui.route = RouteSession
		ui.transcript = next.Messages
		ui.reducer.Hydrate(next.Messages)
		fmt.Fprintln(ui.out, "Forked session:", next.ID)
		return true, nil
	case "/rename":
		if len(parts) < 2 {
			return true, fmt.Errorf("usage: /rename <title>")
		}
		return true, ui.app.RenameSession(strings.TrimSpace(strings.TrimPrefix(line, parts[0])))
	default:
		return ui.app.Command(ctx, line, ui.out)
	}
}

func (ui *UI) handlePickerLine(ctx context.Context, line string) error {
	if ui.picker == nil {
		return nil
	}
	command := strings.TrimSpace(line)
	switch strings.ToLower(command) {
	case "", "down", "/down", "j":
		ui.picker.Move(1)
		return nil
	case "up", "/up", "k":
		ui.picker.Move(-1)
		return nil
	case "esc", "escape", "/cancel", "q":
		ui.picker = nil
		ui.pickerKind = ""
		return nil
	case "enter", "select", "/select":
		return ui.selectPicker(ctx)
	}
	if strings.HasPrefix(command, "/filter ") {
		ui.picker.SetQuery(strings.TrimSpace(strings.TrimPrefix(command, "/filter ")))
		return nil
	}
	if number := atoi(command); number > 0 {
		ui.picker.Index = number - 1
		return ui.selectPicker(ctx)
	}
	ui.picker.SetQuery(command)
	return nil
}

func (ui *UI) selectPicker(ctx context.Context) error {
	item, ok := ui.picker.Selected()
	if !ok {
		return fmt.Errorf("no picker result selected")
	}
	kind := ui.pickerKind
	ui.picker = nil
	ui.pickerKind = ""
	switch kind {
	case "model":
		if err := ui.app.SetModel(item.ID); err != nil {
			return err
		}
	case "variant":
		if err := ui.app.SetVariant(item.ID); err != nil {
			return err
		}
	case "stash":
		index := atoi(item.ID)
		entries := ui.promptStash.Entries()
		if index < 0 || index >= len(entries) {
			return fmt.Errorf("stash entry is no longer available")
		}
		entry := entries[index]
		if err := ui.promptStash.Remove(index); err != nil {
			return err
		}
		ui.editor.Set(entry.Input)
		ui.promptParts = clonePromptParts(entry.Parts)
	case "agent":
		if err := ui.app.SetAgent(item.ID); err != nil {
			return err
		}
	case "session":
		next, err := ui.app.SelectSession(item.ID)
		if err != nil {
			return err
		}
		ui.route = RouteSession
		ui.transcript = next.Messages
		ui.reducer.Hydrate(next.Messages)
	case "command":
		if commandpkg.Canonical(item.ID) == "exit" {
			ui.exitRequested = true
			return nil
		}
		_, err := ui.command(ctx, item.ID)
		return err
	}
	return nil
}

func (ui *UI) draw() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	fmt.Fprint(ui.out, "\033[2J\033[H")
	if ui.viewport == nil {
		ui.viewport = NewViewport(80, 18)
	}
	width, height := terminalSize(ui.terminal)
	ui.viewport.SetSize(width, height-6)
	separator := strings.Repeat("─", maxInt(width, 1))
	if ui.route == RouteHome {
		fmt.Fprintln(ui.out, "YTEAM  Home")
		fmt.Fprintln(ui.out, "")
		fmt.Fprintln(ui.out, "Enter a prompt to start a session.")
		fmt.Fprintln(ui.out, "Example: Inspect this project's structure")
	} else {
		current := ui.app.CurrentSession()
		fmt.Fprintf(ui.out, "YTEAM  Session %s\n", current.ID)
		fmt.Fprintln(ui.out, separator)
		messages := make([]MessageView, 0, len(ui.reducer.Messages))
		for _, message := range ui.reducer.Messages {
			content := message.Content
			if message.Role == "tool" && content == "" {
				content = message.ToolName
			}
			messages = append(messages, MessageView{Role: message.Role, Content: content})
		}
		if len(messages) == 0 {
			for _, message := range ui.transcript {
				messages = append(messages, MessageView{Role: message.Role, Content: message.Content})
			}
		}
		ui.viewport.SetLines(transcriptLines(messages, ui.viewport.Width))
		for _, line := range ui.viewport.Visible() {
			fmt.Fprintln(ui.out, line)
		}
		status := ui.reducer.Status
		if status != "idle" {
			fmt.Fprintf(ui.out, "\nstatus: %s\n", status)
		}
	}
	if ui.picker != nil {
		fmt.Fprintf(ui.out, "\n%s\nSearch: %s\n", ui.picker.Title, ui.picker.Query)
		items := ui.picker.Filtered()
		if len(items) == 0 {
			fmt.Fprintln(ui.out, "  (no results)")
		}
		for index, item := range items {
			marker := "  "
			if index == ui.picker.Index {
				marker = "> "
			}
			fmt.Fprintf(ui.out, "%s%s — %s\n", marker, item.Label, item.Description)
		}
		fmt.Fprintln(ui.out, "Keys: up/down, /filter <text>, enter/select, esc")
	}
	if ui.autocomplete != nil && ui.autocomplete.Visible {
		fmt.Fprintf(ui.out, "\nAutocomplete %s: %s\n", ui.autocomplete.Kind, ui.autocomplete.Query)
		for index, item := range ui.autocomplete.Items {
			marker := "  "
			if index == ui.autocomplete.Index {
				marker = "> "
			}
			fmt.Fprintf(ui.out, "%s%s — %s\n", marker, item.Label, item.Description)
		}
	}
	pending := ui.app.PendingPermissionsForSession(ui.app.CurrentSession().ID)
	if len(pending) > 0 {
		fmt.Fprintf(ui.out, "\nPermission required: %s on %s\n", pending[0].Action, strings.Join(pending[0].Resources, ", "))
		fmt.Fprintln(ui.out, "Press y=once, a=always, n=reject")
	}
	questions := ui.app.PendingQuestions(ui.app.CurrentSession().ID)
	if len(questions) > 0 && len(questions[0].Questions) > 0 {
		request := questions[0]
		ui.prepareQuestionState(request)
		if ui.questionDone {
			fmt.Fprintln(ui.out, "\nAll questions have been answered.")
			fmt.Fprintln(ui.out, "Press enter to submit, esc to cancel")
		} else {
			item := request.Questions[ui.questionAt]
			fmt.Fprintf(ui.out, "\nQuestion: %s (%d/%d)\n", item.Question, ui.questionAt+1, len(request.Questions))
			for index, option := range item.Options {
				marker := " "
				if ui.questionSet[ui.questionAt] != nil && ui.questionSet[ui.questionAt][index] {
					marker = "✓"
				}
				fmt.Fprintf(ui.out, " %s %d. %s — %s\n", marker, index+1, option.Label, option.Description)
			}
			if item.Custom != nil && *item.Custom {
				fmt.Fprintf(ui.out, "  c. custom answer: %s\n", ui.questionText[ui.questionAt])
			}
			if item.Multiple {
				fmt.Fprintln(ui.out, "Select multiple numbers; press enter to continue")
			} else {
				fmt.Fprintln(ui.out, "Type an answer number; press enter to continue")
			}
			if ui.questionMode {
				fmt.Fprintln(ui.out, "Custom answer (type then press enter):", ui.questionText[ui.questionAt])
			}
			fmt.Fprintln(ui.out, "Press esc to go back/reject")
		}
	}
	fmt.Fprintln(ui.out, separator)
	fmt.Fprintf(ui.out, "agent: %s  |  model: %s", ui.app.AgentName(), ui.app.ModelName())
	if variant := ui.app.VariantName(); variant != "" {
		fmt.Fprintf(ui.out, "  |  variant: %s", variant)
	}
	fmt.Fprintln(ui.out, "  |  /help /models /variants /agents /sessions /new /editor /exit")
	if ui.editor != nil {
		fmt.Fprintf(ui.out, "> %s", editorWithCaret(ui.editor))
	} else {
		fmt.Fprint(ui.out, "> ")
	}
}

func (ui *UI) Write(data []byte) (int, error) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.out.Write(data)
}

func IsTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func normalizeLine(value string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "\ufeff"))
}

func normalizePromptContent(content string) string {
	if strings.HasSuffix(content, "\r\n") {
		body := content[:len(content)-2]
		if !strings.ContainsAny(body, "\n\r") {
			return body
		}
	}
	if strings.HasSuffix(content, "\n") {
		body := content[:len(content)-1]
		if !strings.ContainsAny(body, "\n\r") {
			return body
		}
	}
	return content
}
