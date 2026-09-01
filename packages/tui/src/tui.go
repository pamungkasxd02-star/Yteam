package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

type Route string

const (
	RouteHome    Route = "home"
	RouteSession Route = "session"
)

type UI struct {
	app          *runtime.Runtime
	in           io.Reader
	out          io.Writer
	mu           sync.Mutex
	route        Route
	input        string
	palette      bool
	paletteQuery string
	selected     int
	transcript   []session.Message
	picker       *Picker
	pickerKind   string
}

func New(app *runtime.Runtime, in io.Reader, out io.Writer) *UI {
	return &UI{app: app, in: in, out: out, route: RouteHome, transcript: app.CurrentSession().Messages}
}

func (ui *UI) Run(ctx context.Context) error {
	ui.draw()
	scanner := bufio.NewScanner(ui.in)
	for {
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := scanner.Text()
		if line == "\x03" || strings.TrimSpace(line) == "/exit" {
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
		models, err := ui.app.Provider.Models(ctx)
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
		return false, nil
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
	if ui.route == RouteHome {
		fmt.Fprintln(ui.out, "YTEAM  Home")
		fmt.Fprintln(ui.out, "")
		fmt.Fprintln(ui.out, "Tulis permintaan untuk memulai sesi.")
		fmt.Fprintln(ui.out, "Contoh: Periksa struktur proyek ini")
	} else {
		current := ui.app.CurrentSession()
		fmt.Fprintf(ui.out, "YTEAM  Session %s\n", current.ID)
		fmt.Fprintln(ui.out, strings.Repeat("─", 72))
		for _, message := range ui.transcript {
			fmt.Fprintf(ui.out, "%s: %s\n\n", message.Role, message.Content)
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
	fmt.Fprintln(ui.out, strings.Repeat("─", 72))
	fmt.Fprintf(ui.out, "agent: %s  |  model: %s  |  /help /models /agents /sessions /new /exit\n", ui.app.AgentName(), ui.app.ModelName())
	fmt.Fprint(ui.out, "> ")
}

func IsTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
