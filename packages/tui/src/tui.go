package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

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
	promptBusy    bool
	promptDone    chan error
	promptHistory *PromptHistory
	questionID    string
	questionAt    int
	questionSet   map[int]map[int]bool
	questionText  map[int]string
	questionMode  bool
	questionDone  bool
}

func New(app *runtime.Runtime, in io.Reader, out io.Writer) *UI {
	reducer := NewTranscriptReducer()
	current := app.CurrentSession()
	reducer.Hydrate(current.Messages)
	autocomplete := NewAutocomplete()
	for _, item := range app.CommandList() {
		autocomplete.Commands = append(autocomplete.Commands, PickerItem{ID: "/" + item.Name, Label: "/" + item.Name, Description: item.Description})
	}
	for _, item := range []PickerItem{{ID: "/help", Label: "/help", Description: "Show help"}, {ID: "/models", Label: "/models", Description: "Choose a model"}, {ID: "/agents", Label: "/agents", Description: "Choose an agent"}, {ID: "/sessions", Label: "/sessions", Description: "Choose a session"}, {ID: "/new", Label: "/new", Description: "Create a session"}, {ID: "/fork", Label: "/fork", Description: "Fork the current session"}, {ID: "/rename", Label: "/rename", Description: "Rename the current session"}, {ID: "/export", Label: "/export", Description: "Export the current session"}, {ID: "/editor", Label: "/editor", Description: "Open external editor"}, {ID: "/exit", Label: "/exit", Description: "Exit YTEAM"}} {
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
	return &UI{app: app, in: in, out: out, route: RouteHome, transcript: current.Messages, editor: NewEditor(), reducer: reducer, redraw: make(chan struct{}, 1), autocomplete: autocomplete, questionSet: map[int]map[int]bool{}, questionText: map[int]string{}, promptDone: make(chan error, 1), promptHistory: history, viewport: NewViewport(80, 18)}
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
		if line == "\x03" || strings.TrimSpace(line) == "/exit" || strings.TrimSpace(line) == "/quit" {
			return nil
		}
		if ui.picker != nil {
			if err := ui.handlePickerLine(ctx, line); err != nil {
				fmt.Fprintln(ui.out, "error:", err)
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
			ui.transcript = ui.app.CurrentSession().Messages
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
	defer restore()
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
			ui.draw()
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
		if key.Kind == KeyCtrlC {
			ui.app.InterruptSession(ui.app.CurrentSession().ID)
			return nil
		}
		if ui.picker != nil {
			if err := ui.handlePickerKey(ctx, key); err != nil {
				fmt.Fprintln(ui.out, "error:", err)
			}
			ui.draw()
			continue
		}
		if ui.handleQuestionKey(ctx, key) {
			ui.draw()
			continue
		}
		switch key.Kind {
		case KeyText:
			if ui.handlePermissionKey(key) {
				continue
			}
			ui.editor.Insert(key.Text)
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyPaste:
			ui.editor.Insert(key.Text)
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyCtrlJ:
			ui.editor.Newline()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyTab:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.autocomplete.Accept(ui.editor)
				ui.refreshAutocomplete()
			}
		case KeyEnter:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.autocomplete.Accept(ui.editor)
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
				fmt.Fprintln(ui.out, "\nRun masih berjalan. Tekan Ctrl+C untuk membatalkan.")
				ui.draw()
				continue
			}
			if text == "/editor" {
				value, newRestore, editorErr := ui.openEditor(ctx, file, restore, "")
				if editorErr != nil {
					fmt.Fprintln(ui.out, "editor error:", editorErr)
				} else if value != "" {
					ui.editor.Set(normalizePromptContent(value))
				}
				if newRestore != nil {
					restore = newRestore
				}
				ui.draw()
				continue
			}
			ui.editor.AddHistory(content)
			if ui.promptHistory != nil {
				if err := ui.promptHistory.Append(content); err != nil {
					fmt.Fprintln(ui.out, "history error:", err)
				}
			}
			ui.editor.Reset()
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
				if text == "/exit" || text == "/quit" {
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
			go func() {
				ui.promptDone <- ui.app.Prompt(ctx, content, ui)
			}()
		case KeyBackspace:
			ui.editor.Backspace()
			ui.resetPromptHistoryNavigation()
			ui.refreshAutocomplete()
		case KeyDelete:
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
			if ui.promptHistory != nil {
				if value, ok := ui.promptHistory.Move(-1, ui.editor.String()); ok {
					ui.editor.Set(value)
				}
			} else if ui.editor.Cursor() == lineStart(ui.editor.value, ui.editor.cursor) && lineStart(ui.editor.value, ui.editor.cursor) == 0 {
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
			if ui.promptHistory != nil {
				if value, ok := ui.promptHistory.Move(1, ui.editor.String()); ok {
					ui.editor.Set(value)
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
					ui.editor.Set(value)
				}
			} else {
				ui.editor.HistoryUp()
			}
		case KeyEscape:
			if ui.autocomplete != nil && ui.autocomplete.Visible {
				ui.autocomplete.Close()
			} else {
				ui.editor.Reset()
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

func (ui *UI) resetPromptHistoryNavigation() {
	if ui.promptHistory != nil {
		ui.promptHistory.ResetNavigation()
	}
}

func (ui *UI) isPromptCommand(text string) bool {
	parts := strings.Fields(text)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return false
	}
	name := strings.TrimPrefix(parts[0], "/")
	if name == "editor" {
		return true
	}
	_, ok := ui.app.Commands[name]
	return ok
}

func (ui *UI) openEditor(ctx context.Context, file *os.File, restore func(), value string) (string, func(), error) {
	restore()
	content, editorErr := openExternalEditor(ctx, value, ui.app.Root, file, ui.out, ui.out)
	newRestore, rawErr := enableRaw(file)
	if editorErr != nil {
		if rawErr != nil {
			return "", nil, fmt.Errorf("%w; terminal restore failed: %v", editorErr, rawErr)
		}
		return "", newRestore, editorErr
	}
	if rawErr != nil {
		return "", nil, rawErr
	}
	return content, newRestore, nil
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
	case "/palette":
		ui.palette = true
		ui.paletteQuery = ""
		ui.selected = 0
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
		ui.picker, ui.pickerKind = NewPicker("Pilih model", items), "model"
		return true, nil
	case "/agent", "/agents":
		if len(parts) < 2 {
			ui.picker, ui.pickerKind = NewPicker("Pilih agent", []PickerItem{{ID: "build", Label: "build", Description: "Implement changes and run tools"}, {ID: "plan", Label: "plan", Description: "Inspect the project and propose a plan"}}), "agent"
			return true, nil
		}
		if err := ui.app.SetAgent(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(ui.out, "Agent aktif:", ui.app.AgentName())
		return true, nil
	case "/model":
		if len(parts) < 2 {
			fmt.Fprintln(ui.out, "Model aktif:", ui.app.ModelName())
			return true, nil
		}
		if err := ui.app.SetModel(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(ui.out, "Model aktif:", ui.app.ModelName())
		return true, nil
	case "/sessions":
		items, err := ui.app.ListSessions()
		if err != nil {
			return true, err
		}
		options := make([]PickerItem, 0, len(items))
		for _, item := range items {
			options = append(options, PickerItem{ID: item.ID, Label: item.Title, Description: item.Directory})
		}
		ui.picker, ui.pickerKind = NewPicker("Pilih session", options), "session"
		return true, nil
	case "/new", "/clear":
		next, err := ui.app.NewSession()
		if err != nil {
			return true, err
		}
		ui.route = RouteHome
		ui.transcript = nil
		fmt.Fprintln(ui.out, "Session baru:", next.ID)
		return true, nil
	case "/fork":
		next, err := ui.app.ForkSession()
		if err != nil {
			return true, err
		}
		ui.route = RouteSession
		ui.transcript = next.Messages
		fmt.Fprintln(ui.out, "Fork session:", next.ID)
		return true, nil
	case "/rename":
		if len(parts) < 2 {
			return true, fmt.Errorf("penggunaan: /rename <judul>")
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

func (ui *UI) selectPicker(_ context.Context) error {
	item, ok := ui.picker.Selected()
	if !ok {
		return fmt.Errorf("tidak ada hasil pilihan")
	}
	kind := ui.pickerKind
	ui.picker = nil
	ui.pickerKind = ""
	switch kind {
	case "model":
		if err := ui.app.SetModel(item.ID); err != nil {
			return err
		}
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
	if ui.route == RouteHome {
		fmt.Fprintln(ui.out, "YTEAM  Home")
		fmt.Fprintln(ui.out, "")
		fmt.Fprintln(ui.out, "Tulis permintaan untuk memulai sesi.")
		fmt.Fprintln(ui.out, "Contoh: Periksa struktur proyek ini")
	} else {
		current := ui.app.CurrentSession()
		fmt.Fprintf(ui.out, "YTEAM  Session %s\n", current.ID)
		fmt.Fprintln(ui.out, strings.Repeat("─", 72))
		messages := make([]MessageView, 0, len(ui.transcript))
		for _, message := range ui.transcript {
			messages = append(messages, MessageView{Role: message.Role, Content: message.Content})
		}
		ui.viewport.SetLines(transcriptLines(messages, ui.viewport.Width))
		for _, line := range ui.viewport.Visible() {
			fmt.Fprintln(ui.out, line)
		}
	}
	if ui.picker != nil {
		fmt.Fprintf(ui.out, "\n%s\nCari: %s\n", ui.picker.Title, ui.picker.Query)
		items := ui.picker.Filtered()
		if len(items) == 0 {
			fmt.Fprintln(ui.out, "  (tidak ada hasil)")
		}
		for index, item := range items {
			marker := "  "
			if index == ui.picker.Index {
				marker = "> "
			}
			fmt.Fprintf(ui.out, "%s%s — %s\n", marker, item.Label, item.Description)
		}
		fmt.Fprintln(ui.out, "Ketik: up/down, /filter <teks>, enter/select, esc")
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
		fmt.Fprintf(ui.out, "\nIzin diperlukan: %s pada %s\n", pending[0].Action, strings.Join(pending[0].Resources, ", "))
		fmt.Fprintln(ui.out, "Tekan y=sekali, a=selalu, n=tolak")
	}
	questions := ui.app.PendingQuestions(ui.app.CurrentSession().ID)
	if len(questions) > 0 && len(questions[0].Questions) > 0 {
		request := questions[0]
		ui.prepareQuestionState(request)
		if ui.questionDone {
			fmt.Fprintln(ui.out, "\nSemua pertanyaan sudah dipilih.")
			fmt.Fprintln(ui.out, "Tekan enter untuk mengirim jawaban, esc untuk membatalkan")
		} else {
			item := request.Questions[ui.questionAt]
			fmt.Fprintf(ui.out, "\nPertanyaan: %s (%d/%d)\n", item.Question, ui.questionAt+1, len(request.Questions))
			for index, option := range item.Options {
				marker := " "
				if ui.questionSet[ui.questionAt] != nil && ui.questionSet[ui.questionAt][index] {
					marker = "✓"
				}
				fmt.Fprintf(ui.out, " %s %d. %s — %s\n", marker, index+1, option.Label, option.Description)
			}
			if item.Custom != nil && *item.Custom {
				fmt.Fprintf(ui.out, "  c. jawaban custom: %s\n", ui.questionText[ui.questionAt])
			}
			if item.Multiple {
				fmt.Fprintln(ui.out, "Pilih beberapa nomor; tekan enter untuk lanjut")
			} else {
				fmt.Fprintln(ui.out, "Ketik nomor jawaban; tekan enter untuk lanjut")
			}
			if ui.questionMode {
				fmt.Fprintln(ui.out, "Jawaban custom (ketik lalu enter):", ui.questionText[ui.questionAt])
			}
			fmt.Fprintln(ui.out, "Tekan esc untuk kembali/menolak")
		}
	}
	fmt.Fprintln(ui.out, strings.Repeat("─", 72))
	fmt.Fprintf(ui.out, "agent: %s  |  model: %s  |  /help /models /agents /sessions /new /exit\n", ui.app.AgentName(), ui.app.ModelName())
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
