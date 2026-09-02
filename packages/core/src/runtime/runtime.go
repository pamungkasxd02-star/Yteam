package runtime

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/event"
	gitstatus "github.com/pamungkasxd02-star/Yteam/packages/core/src/git"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/question"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session/runner"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/tool"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
	"github.com/pamungkasxd02-star/Yteam/packages/skill/src"
)

type Runtime struct {
	mu           sync.RWMutex
	Config       config.Config
	Root         string
	Store        *session.Store
	Session      *session.Session
	Provider     *provider.Client
	Runner       *runner.Runner
	Events       *event.Journal
	Coordinator  *runner.Coordinator
	Permissions  *permission.Engine
	Agent        string
	Model        string
	MCPStatus    func() any
	SkillContext string
	LSPStatus    func() any
	LSPExecute   func(context.Context, any) (any, error)
	Inputs       *session.InputQueue
	Questions    *question.Manager
	runMu        sync.Mutex
	cancelRuns   map[string]context.CancelFunc
}

func New(cfg config.Config, root string, store *session.Store, current *session.Session, client *provider.Client) *Runtime {
	permissions := permission.New([]permission.Rule{
		{Action: "read", Resource: "*", Effect: permission.Allow},
		{Action: "list", Resource: "*", Effect: permission.Allow},
	})
	questions := question.NewManager()
	tools := tool.Builtins(permissions)
	tools.Add(tool.Question{Manager: questionsAdapter{manager: questions}})
	return &Runtime{
		Config:      cfg,
		Root:        root,
		Store:       store,
		Session:     current,
		Provider:    client,
		Runner:      &runner.Runner{Provider: client, Store: store, Tools: tools},
		Coordinator: runner.NewCoordinator(),
		Permissions: permissions,
		Agent:       "build",
		Model:       cfg.Model,
		Inputs:      store.Inputs(),
		cancelRuns:  map[string]context.CancelFunc{},
		Questions:   questions,
	}
}

type questionsAdapter struct{ manager *question.Manager }

func (a questionsAdapter) AskQuestion(sessionID string, items []schema.QuestionInfo, toolRef *schema.QuestionToolRef) (schema.QuestionRequest, error) {
	return a.manager.Ask(sessionID, items, toolRef)
}
func (a questionsAdapter) AwaitQuestion(ctx context.Context, id string) ([]schema.QuestionAnswer, error) {
	return a.manager.Await(ctx, id)
}

func (r *Runtime) AttachEvents(journal *event.Journal) {
	r.mu.Lock()
	r.Events = journal
	r.mu.Unlock()
}

func (r *Runtime) EventJournal() *event.Journal {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Events
}

func (r *Runtime) RunnerTools() []schema.ToolDefinition {
	if r.Runner == nil || r.Runner.Tools == nil {
		return nil
	}
	return r.Runner.Tools.Definitions()
}

func (r *Runtime) SetApproval(approve func(permission.Request) permission.Reply) {
	if r.Runner == nil || r.Runner.Tools == nil {
		return
	}
	r.Runner.Tools.SetApproval(approve)
}

func (r *Runtime) ActiveRuns() []string {
	if r.Coordinator == nil {
		return nil
	}
	return r.Coordinator.Active()
}

func (r *Runtime) SetAgent(name string) error {
	name = strings.TrimSpace(name)
	if name != "build" && name != "plan" {
		return fmt.Errorf("unknown agent: %s", name)
	}
	r.mu.Lock()
	r.Agent = name
	r.mu.Unlock()
	return nil
}

func (r *Runtime) AgentName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Agent
}

func (r *Runtime) Agents() []map[string]string {
	return []map[string]string{
		{"name": "build", "description": "Implement changes and run tools", "mode": "build"},
		{"name": "plan", "description": "Inspect the project and propose a plan", "mode": "plan"},
	}
}

func (r *Runtime) SetModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is empty")
	}
	r.mu.Lock()
	r.Model = model
	r.Config.Model = model
	r.mu.Unlock()
	return nil
}

func (r *Runtime) ModelName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Model != "" {
		return r.Model
	}
	return r.Config.Model
}

func (r *Runtime) GitStatus(ctx context.Context) (gitstatus.Status, error) {
	return gitstatus.Read(ctx, r.Root)
}

func (r *Runtime) GitDiff(ctx context.Context) (string, error) {
	return gitstatus.Diff(ctx, r.Root)
}

func (r *Runtime) GitLog(ctx context.Context, count int) (string, error) {
	return gitstatus.Log(ctx, r.Root, count)
}

func (r *Runtime) Skills() ([]skill.Skill, error) {
	items, err := skill.Discover(r.Root)
	if err == nil {
		r.mu.Lock()
		r.SkillContext = skill.SystemContext(items)
		r.mu.Unlock()
	}
	return items, err
}

func (r *Runtime) SystemPrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.SkillContext == "" {
		return r.Config.SystemPrompt
	}
	if r.Config.SystemPrompt == "" {
		return r.SkillContext
	}
	return r.Config.SystemPrompt + "\n\n" + r.SkillContext
}

func (r *Runtime) AddExternalTool(caller tool.ExternalCaller, name, description string, parameters map[string]any) error {
	return r.AddExternalToolNamed(caller, name, name, description, parameters)
}

func (r *Runtime) AddExternalToolNamed(caller tool.ExternalCaller, displayName, remoteName, description string, parameters map[string]any) error {
	if caller == nil {
		return fmt.Errorf("external caller is nil")
	}
	if strings.TrimSpace(displayName) == "" || strings.TrimSpace(remoteName) == "" {
		return fmt.Errorf("external tool name is empty")
	}
	if r.Runner == nil || r.Runner.Tools == nil {
		return fmt.Errorf("tool registry is not configured")
	}
	r.Runner.Tools.Add(tool.ExternalTool{Caller: caller, ToolName: displayName, RemoteName: remoteName, ToolDescription: description, ToolParameters: parameters})
	return nil
}

func (r *Runtime) SetMCPStatus(status func() any) { r.mu.Lock(); r.MCPStatus = status; r.mu.Unlock() }

func (r *Runtime) MCP() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.MCPStatus == nil {
		return []any{}
	}
	return r.MCPStatus()
}

func (r *Runtime) SetLSPStatus(status func() any) { r.mu.Lock(); r.LSPStatus = status; r.mu.Unlock() }

func (r *Runtime) LSP() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.LSPStatus == nil {
		return []any{}
	}
	return r.LSPStatus()
}

func (r *Runtime) SetLSPExecute(execute func(context.Context, any) (any, error)) {
	r.mu.Lock()
	r.LSPExecute = execute
	r.mu.Unlock()
}
func (r *Runtime) ExecuteLSP(ctx context.Context, input any) (any, error) {
	r.mu.RLock()
	execute := r.LSPExecute
	r.mu.RUnlock()
	if execute == nil {
		return nil, fmt.Errorf("LSP is not configured")
	}
	return execute(ctx, input)
}

func (r *Runtime) PendingPermissions() []permission.Request {
	if r.Permissions == nil {
		return nil
	}
	return r.Permissions.Pending()
}

func (r *Runtime) PendingPermissionsForSession(sessionID string) []permission.Request {
	items := r.PendingPermissions()
	result := make([]permission.Request, 0, len(items))
	for _, item := range items {
		if item.SessionID == sessionID {
			result = append(result, item)
		}
	}
	return result
}

func (r *Runtime) ReplyPermission(id string, reply permission.Reply) error {
	if r.Permissions == nil {
		return fmt.Errorf("permission engine is not configured")
	}
	return r.Permissions.Reply(id, reply)
}

func (r *Runtime) ReplyPermissionForSession(sessionID, id string, reply permission.Reply) error {
	request, ok := r.Permissions.Get(id)
	if !ok || request.SessionID != sessionID {
		return fmt.Errorf("permission request not found for session")
	}
	return r.ReplyPermission(id, reply)
}

func (r *Runtime) PendingQuestions(sessionID string) []schema.QuestionRequest {
	if r.Questions == nil {
		return nil
	}
	return r.Questions.Pending(sessionID)
}

func (r *Runtime) AskQuestion(sessionID string, items []schema.QuestionInfo, toolRef *schema.QuestionToolRef) (schema.QuestionRequest, error) {
	if r.Questions == nil {
		r.Questions = question.NewManager()
	}
	request, err := r.Questions.Ask(sessionID, items, toolRef)
	if err == nil && r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventQuestionAsked, sessionID, map[string]any{"request_id": request.ID})
	}
	return request, err
}

func (r *Runtime) ReplyQuestion(ctx context.Context, sessionID, id string, answers []schema.QuestionAnswer) error {
	if r.Questions == nil {
		return question.ErrNotFound
	}
	err := r.Questions.Reply(ctx, sessionID, id, answers)
	if err == nil && r.Events != nil {
		_, _ = r.Events.Publish(ctx, schema.EventQuestionReplied, sessionID, map[string]any{"request_id": id})
	}
	return err
}

func (r *Runtime) RejectQuestion(ctx context.Context, sessionID, id string) error {
	if r.Questions == nil {
		return question.ErrNotFound
	}
	err := r.Questions.Reject(ctx, sessionID, id)
	if err == nil && r.Events != nil {
		_, _ = r.Events.Publish(ctx, schema.EventQuestionRejected, sessionID, map[string]any{"request_id": id})
	}
	return err
}

func (r *Runtime) PendingInputs(sessionID string) []session.Input {
	if r.Inputs == nil {
		return nil
	}
	return r.Inputs.Pending(sessionID)
}

func (r *Runtime) AdmitInput(sessionID, content string, delivery session.Delivery) (session.Input, error) {
	if r.Inputs == nil {
		r.Inputs = session.NewInputQueue()
	}
	item, err := r.Inputs.Admit(sessionID, content, delivery)
	if err != nil {
		return session.Input{}, err
	}
	if r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventPromptAdmitted, sessionID, map[string]any{
			"input_id": item.ID, "content": content, "delivery": string(delivery),
		})
	}
	return item, nil
}

func (r *Runtime) PromoteInputs(sessionID string) []session.Input {
	if r.Inputs == nil {
		return nil
	}
	items, _ := r.Inputs.Promote(sessionID)
	return items
}

func (r *Runtime) InterruptSession(sessionID string) {
	if r.Inputs != nil {
		r.Inputs.Interrupt(sessionID)
	}
	r.runMu.Lock()
	cancel := r.cancelRuns[sessionID]
	r.runMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if r.Coordinator != nil {
		_ = r.Coordinator.Interrupt(context.Background(), sessionID)
	}
}

func (r *Runtime) CurrentSession() session.Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Session == nil {
		return session.Session{}
	}
	copy := *r.Session
	copy.Messages = append([]session.Message(nil), r.Session.Messages...)
	return copy
}

func (r *Runtime) ListSessions() ([]session.Session, error) { return r.Store.List() }

func (r *Runtime) SelectSession(id string) (*session.Session, error) {
	next, err := r.Store.Load(id)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	return next, nil
}

func (r *Runtime) NewSession() (*session.Session, error) {
	next, err := r.Store.New()
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	return next, nil
}

func (r *Runtime) RenameSession(title string) error {
	r.mu.RLock()
	current := r.Session
	r.mu.RUnlock()
	if current == nil {
		return fmt.Errorf("session is not initialized")
	}
	next, err := r.Store.Rename(current.ID, title)
	if err != nil {
		return err
	}
	r.SwitchSession(next)
	return nil
}

func (r *Runtime) ForkSession() (*session.Session, error) {
	r.mu.RLock()
	current := r.Session
	r.mu.RUnlock()
	if current == nil {
		return nil, fmt.Errorf("session is not initialized")
	}
	next, err := r.Store.Fork(current.ID)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	return next, nil
}

func (r *Runtime) CompactSession(summary string, keep int) (*session.Compaction, error) {
	current := r.CurrentSession()
	compaction, err := r.Store.CompactMessages(current.ID, summary, keep)
	if err != nil {
		return nil, err
	}
	next, err := r.Store.Load(current.ID)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	if r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventCompactionEnded, current.ID, map[string]any{"summary": compaction.Summary})
	}
	return compaction, nil
}

func (r *Runtime) StageRevert(messageID, diff string) (*session.Session, error) {
	current := r.CurrentSession()
	next, err := r.Store.StageRevert(current.ID, messageID, diff)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	if r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventRevertStaged, current.ID, map[string]any{"message_id": messageID, "diff": diff})
	}
	return next, nil
}
func (r *Runtime) ClearRevert() (*session.Session, error) {
	current := r.CurrentSession()
	next, err := r.Store.ClearRevert(current.ID)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	if r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventRevertCleared, current.ID, nil)
	}
	return next, nil
}
func (r *Runtime) CommitRevert() (*session.Session, error) {
	current := r.CurrentSession()
	messageID := ""
	if current.Revert != nil {
		messageID = current.Revert.MessageID
	}
	next, err := r.Store.CommitRevert(current.ID)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	if r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventRevertCommitted, current.ID, map[string]any{"message_id": messageID})
	}
	return next, nil
}

// Command handles the local command subset shared by the REPL and non-TUI CLI.
// A command is handled locally when it starts with '/', so it never consumes
// provider quota by accident.
func (r *Runtime) Command(ctx context.Context, input string, out io.Writer) (bool, error) {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return false, nil
	}
	switch parts[0] {
	case "/help":
		r.Help(out)
	case "/status":
		r.Status(out)
	case "/history":
		r.History(out)
	case "/sessions":
		items, err := r.ListSessions()
		if err != nil {
			return true, err
		}
		for _, item := range items {
			fmt.Fprintf(out, "%s\t%s\n", item.ID, item.Title)
		}
	case "/models":
		items, err := r.Provider.Models(ctx)
		if err != nil {
			return true, err
		}
		for _, item := range items {
			fmt.Fprintln(out, item.ID)
		}
	case "/agent", "/agents":
		if len(parts) < 2 {
			fmt.Fprintln(out, "agent aktif:", r.AgentName(), "(build, plan)")
			return true, nil
		}
		if err := r.SetAgent(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(out, "agent aktif:", r.AgentName())
	case "/model":
		if len(parts) < 2 {
			fmt.Fprintln(out, "model aktif:", r.ModelName())
			return true, nil
		}
		if err := r.SetModel(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(out, "model aktif:", r.ModelName())
	case "/clear", "/new":
		next, err := r.NewSession()
		if err != nil {
			return true, err
		}
		fmt.Fprintln(out, "Session baru:", next.ID)
	case "/fork":
		next, err := r.ForkSession()
		if err != nil {
			return true, err
		}
		fmt.Fprintln(out, "Fork session:", next.ID)
	case "/rename":
		if len(parts) < 2 {
			fmt.Fprintln(out, "Penggunaan: /rename <judul>")
			return true, nil
		}
		if err := r.RenameSession(strings.TrimSpace(strings.TrimPrefix(input, parts[0]))); err != nil {
			return true, err
		}
	case "/export":
		format := "md"
		if len(parts) > 1 {
			format = parts[1]
		}
		current := r.CurrentSession()
		if format == "json" {
			data, err := r.Store.ExportJSON(current.ID)
			if err != nil {
				return true, err
			}
			_, err = out.Write(data)
			return true, err
		}
		data, err := r.Store.ExportMarkdown(current.ID)
		if err != nil {
			return true, err
		}
		_, err = io.WriteString(out, data)
		return true, err
	default:
		fmt.Fprintf(out, "Perintah tidak dikenal: %s\n", parts[0])
	}
	return true, nil
}

func (r *Runtime) SwitchSession(next *session.Session) {
	r.mu.Lock()
	r.Session = next
	r.mu.Unlock()
}

func (r *Runtime) Prompt(ctx context.Context, text string, out io.Writer) error {
	return r.PromptDelivery(ctx, text, session.DeliveryQueue, out)
}

func (r *Runtime) PromptDelivery(ctx context.Context, text string, delivery session.Delivery, out io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	r.mu.RLock()
	current := r.Session
	r.mu.RUnlock()
	if current == nil {
		return fmt.Errorf("session is not initialized")
	}
	if r.Inputs == nil {
		r.Inputs = session.NewInputQueue()
	}
	input, err := r.AdmitInput(current.ID, text, delivery)
	if err != nil {
		return err
	}
	if _, _, err := r.Inputs.PromoteByID(input.ID); err != nil {
		return err
	}
	user := session.Message{ID: session.NewMessageID(), Role: "user", Content: text}
	if err := r.Store.Append(current.ID, user); err != nil {
		return err
	}
	current.Messages = append(current.Messages, user)
	options := runner.RunOptions{OnText: func(value string) {
		_, _ = io.WriteString(out, value)
		if r.Events != nil {
			_, _ = r.Events.Publish(ctx, schema.EventTextDelta, current.ID, map[string]any{"content": value})
		}
	}, OnToolStart: func(call schema.ToolCall) {
		if r.Events != nil {
			_, _ = r.Events.Publish(ctx, schema.EventToolStarted, current.ID, map[string]any{"name": call.Name, "call_id": call.ID})
		}
	}, OnTool: func(call schema.ToolCall, result string, err error) {
		if r.Events != nil {
			_, _ = r.Events.Publish(ctx, schema.EventToolFinished, current.ID, map[string]any{"name": call.Name, "error": errorText(err)})
		}
		if err != nil {
			_, _ = fmt.Fprintf(out, "\n[tool %s error: %v]\n", call.Name, err)
			return
		}
		_, _ = fmt.Fprintf(out, "\n[tool %s selesai]\n", call.Name)
		_ = result
	}}
	if r.Coordinator == nil {
		r.Coordinator = runner.NewCoordinator()
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.runMu.Lock()
	r.cancelRuns[current.ID] = cancel
	r.runMu.Unlock()
	defer func() {
		cancel()
		r.Inputs.ClearInterrupt(current.ID)
		r.runMu.Lock()
		delete(r.cancelRuns, current.ID)
		r.runMu.Unlock()
	}()
	if err := r.Coordinator.Run(runCtx, current.ID, func(runCtx context.Context) error {
		return r.Runner.RunWithOptions(runCtx, current, r.ModelName(), r.SystemPrompt(), options)
	}); err != nil {
		return err
	}
	_, err = io.WriteString(out, "\n")
	return err
}
func (r *Runtime) Help(out io.Writer) {
	fmt.Fprintln(out, "Perintah YTEAM:")
	fmt.Fprintln(out, "  /help       tampilkan bantuan")
	fmt.Fprintln(out, "  /status     tampilkan status proyek")
	fmt.Fprintln(out, "  /models     ambil daftar model")
	fmt.Fprintln(out, "  /history    tampilkan riwayat")
	fmt.Fprintln(out, "  /clear      buat session baru")
	fmt.Fprintln(out, "  /exit       keluar")
}
func (r *Runtime) Status(out io.Writer) {
	fmt.Fprintf(out, "proyek: %s\nmodel: %s\nsession: %s\njudul: %s\n", r.Root, r.Config.Model, r.Session.ID, r.Session.Title)
}
func (r *Runtime) History(out io.Writer) {
	for _, item := range r.Session.Messages {
		fmt.Fprintf(out, "[%s] %s: %s\n", item.CreatedAt, item.Role, item.Content)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
