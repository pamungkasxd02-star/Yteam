package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	agentpkg "github.com/pamungkasxd02-star/Yteam/packages/agent/src"
	commandpkg "github.com/pamungkasxd02-star/Yteam/packages/command/src"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/event"
	gitstatus "github.com/pamungkasxd02-star/Yteam/packages/core/src/git"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/question"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session/runner"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/snapshot"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/tool"
	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
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
	Variant      string
	MCPStatus    func() any
	SkillContext string
	LSPStatus    func() any
	LSPExecute   func(context.Context, any) (any, error)
	PluginStatus func() any
	Inputs       *session.InputQueue
	Questions    *question.Manager
	Snapshot     *snapshot.Service
	Commands     map[string]commandpkg.Info
	runMu        sync.Mutex
	cancelRuns   map[string]context.CancelFunc
}

func New(cfg config.Config, root string, store *session.Store, current *session.Session, client *provider.Client) *Runtime {
	defaultRules := []permission.Rule{
		{Action: "read", Resource: "*", Effect: permission.Allow},
		{Action: "list", Resource: "*", Effect: permission.Allow},
	}
	permissions, permissionErr := permission.Open(cfg.Home, defaultRules)
	if permissionErr != nil {
		permissions = permission.New(defaultRules)
	}
	questions, questionErr := question.OpenManager(cfg.Home)
	if questionErr != nil {
		questions = question.NewManager()
	}
	snapshots, err := snapshot.New(cfg.Home, root)
	if err != nil {
		// Runtime construction historically had no error return. Keep the
		// constructor total and let revert report the missing service clearly.
		snapshots = nil
	}
	tools := tool.Builtins(permissions)
	tools.Add(tool.Question{Manager: questionsAdapter{manager: questions}})
	commands, _ := commandpkg.Discover(root)
	commandMap := make(map[string]commandpkg.Info, len(commands))
	for _, item := range commands {
		commandMap[item.Name] = item
	}
	selectedAgent := firstAgent(cfg.Agent)
	if current != nil && current.Agent != "" {
		selectedAgent = firstAgent(current.Agent)
	}
	if current != nil {
		current.Agent = selectedAgent
		if _, err := store.SetAgent(current.ID, selectedAgent); err != nil {
			// Runtime construction remains non-failing for compatibility; the
			// in-memory selection still applies and explicit switches retry it.
		}
	}
	return &Runtime{
		Config:      cfg,
		Root:        root,
		Store:       store,
		Session:     current,
		Provider:    client,
		Runner:      &runner.Runner{Provider: client, Store: store, Tools: tools, Agent: selectedAgent},
		Coordinator: runner.NewCoordinator(),
		Permissions: permissions,
		Agent:       selectedAgent,
		Model:       cfg.Model,
		Inputs:      store.Inputs(),
		cancelRuns:  map[string]context.CancelFunc{},
		Questions:   questions,
		Snapshot:    snapshots,
		Commands:    commandMap,
	}
}

type questionsAdapter struct{ manager *question.Manager }

func firstAgent(name string) string {
	if _, ok := agentpkg.Find(name); ok {
		return name
	}
	return "build"
}

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
	return r.Runner.ToolDefinitions()
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
	if _, ok := agentpkg.Find(name); !ok {
		return fmt.Errorf("unknown agent: %s", name)
	}
	r.mu.Lock()
	r.Agent = name
	r.Config.Agent = name
	if r.Runner != nil {
		r.Runner.Agent = name
	}
	current := r.Session
	r.mu.Unlock()
	if current != nil {
		next, err := r.Store.SetAgent(current.ID, name)
		if err != nil {
			return err
		}
		r.SwitchSession(next)
	}
	return nil
}

func (r *Runtime) AgentName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Agent
}

func (r *Runtime) Agents() []map[string]string {
	items := agentpkg.Builtins()
	result := make([]map[string]string, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]string{"name": item.Name, "description": item.Description, "mode": item.Mode, "prompt": item.Prompt, "tools": strings.Join(item.Tools, ",")})
	}
	return result
}

func (r *Runtime) CommandList() []commandpkg.Info {
	items := make([]commandpkg.Info, 0, len(r.Commands))
	for _, item := range r.Commands {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (r *Runtime) AgentPrompt() string {
	item, ok := agentpkg.Find(r.AgentName())
	if !ok {
		return ""
	}
	return item.Prompt
}

func (r *Runtime) SystemPromptFor(agentName string) string {
	r.mu.RLock()
	base, skills := r.Config.SystemPrompt, r.SkillContext
	r.mu.RUnlock()
	item, _ := agentpkg.Find(agentName)
	return strings.TrimSpace(strings.Join([]string{base, item.Prompt, skills}, "\n\n"))
}

func (r *Runtime) SetModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("model is empty")
	}
	r.mu.Lock()
	r.Model = model
	r.Config.Model = model
	r.Variant = ""
	r.mu.Unlock()
	return nil
}

func (r *Runtime) SetVariant(variant string) error {
	variant = strings.TrimSpace(variant)
	if variant == "" {
		r.mu.Lock()
		r.Variant = ""
		r.mu.Unlock()
		return nil
	}
	if r.Provider == nil {
		return fmt.Errorf("provider is not configured")
	}
	model, err := r.Provider.Catalog().Find(context.Background(), r.ModelName())
	if err != nil {
		return err
	}
	if len(model.Variants) > 0 {
		if _, ok := model.Variants[variant]; !ok {
			return fmt.Errorf("unknown variant %q for model %q", variant, model.ID)
		}
	}
	r.mu.Lock()
	r.Variant = variant
	r.mu.Unlock()
	return nil
}

func (r *Runtime) VariantName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Variant
}

func (r *Runtime) Variants(ctx context.Context) ([]string, error) {
	if r.Provider == nil {
		return nil, fmt.Errorf("provider is not configured")
	}
	model, err := r.Provider.Catalog().Find(ctx, r.ModelName())
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(model.Variants))
	for name := range model.Variants {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func (r *Runtime) ModelName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Model != "" {
		return r.Model
	}
	return r.Config.Model
}

func (r *Runtime) Models(ctx context.Context) ([]protocol.Model, error) {
	if r.Provider == nil {
		return nil, fmt.Errorf("provider is not configured")
	}
	return r.Provider.Catalog().List(ctx)
}

func (r *Runtime) ProviderUsage() provider.UsageTotals {
	if r.Provider == nil {
		return provider.UsageTotals{}
	}
	return r.Provider.Usage()
}

func (r *Runtime) ProviderUsageByModel() map[string]provider.UsageTotals {
	if r.Provider == nil {
		return map[string]provider.UsageTotals{}
	}
	return r.Provider.UsageByModel()
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
	base, skills, name := r.Config.SystemPrompt, r.SkillContext, r.Agent
	r.mu.RUnlock()
	item, _ := agentpkg.Find(name)
	return strings.TrimSpace(strings.Join([]string{base, item.Prompt, skills}, "\n\n"))
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

func (r *Runtime) SetPluginStatus(status func() any) {
	r.mu.Lock()
	r.PluginStatus = status
	r.mu.Unlock()
}
func (r *Runtime) Plugins() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.PluginStatus == nil {
		return []any{}
	}
	return r.PluginStatus()
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
		questions, err := question.OpenManager(r.Config.Home)
		if err != nil {
			r.Questions = question.NewManager()
		} else {
			r.Questions = questions
		}
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
	if current, err := r.Store.SetRunState(sessionID, session.RunInterrupted, 0, context.Canceled.Error()); err == nil {
		r.mu.Lock()
		if r.Session != nil && r.Session.ID == sessionID {
			r.Session = current
		}
		r.mu.Unlock()
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
		_, _ = r.Events.Publish(context.Background(), schema.EventCompactionEnded, current.ID, map[string]any{"summary": compaction.Summary, "epoch": compaction.Epoch, "token_estimate_before": compaction.TokenEstimateBefore, "token_estimate_after": compaction.TokenEstimateAfter})
	}
	return compaction, nil
}

func (r *Runtime) StageRevert(messageID, diff string) (*session.Session, error) {
	current := r.CurrentSession()
	stored, err := r.Store.Load(current.ID)
	if err != nil {
		return nil, err
	}
	snapshotID := ""
	found := false
	for _, message := range stored.Messages {
		if message.ID == messageID {
			found = true
			snapshotID = message.SnapshotID
			break
		}
	}
	if !found {
		return nil, session.ErrMessageNotFound
	}
	if diff == "" && snapshotID != "" && r.Snapshot != nil {
		if generated, err := r.Snapshot.Diff(snapshotID); err == nil {
			diff = generated
		}
	}
	next, err := r.Store.StageRevertWithSnapshot(current.ID, messageID, diff, snapshotID)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	if r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventRevertStaged, current.ID, map[string]any{"message_id": messageID, "diff": diff, "snapshot_id": snapshotID})
	}
	return next, nil
}

func (r *Runtime) CaptureSnapshot() (*snapshot.Snapshot, error) {
	if r.Snapshot == nil {
		return nil, fmt.Errorf("snapshot service is not configured")
	}
	return r.Snapshot.Capture()
}

func (r *Runtime) DiffSnapshot(id string) (string, error) {
	if r.Snapshot == nil {
		return "", fmt.Errorf("snapshot service is not configured")
	}
	return r.Snapshot.Diff(id)
}
func (r *Runtime) ClearRevert() (*session.Session, error) {
	current := r.CurrentSession()
	oldSnapshot := ""
	if current.Revert != nil {
		oldSnapshot = current.Revert.Snapshot
	}
	next, err := r.Store.ClearRevert(current.ID)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	if oldSnapshot != "" && r.Snapshot != nil {
		_ = r.Snapshot.Remove(oldSnapshot)
	}
	if r.Events != nil {
		_, _ = r.Events.Publish(context.Background(), schema.EventRevertCleared, current.ID, nil)
	}
	return next, nil
}
func (r *Runtime) CommitRevert() (*session.Session, error) {
	current := r.CurrentSession()
	messageID := ""
	snapshotID := ""
	if current.Revert != nil {
		messageID = current.Revert.MessageID
		snapshotID = current.Revert.Snapshot
	}
	if snapshotID != "" {
		if r.Snapshot == nil {
			return nil, fmt.Errorf("snapshot service is not configured")
		}
		if err := r.Snapshot.Restore(snapshotID); err != nil {
			return nil, err
		}
	}
	next, err := r.Store.CommitRevert(current.ID)
	if err != nil {
		return nil, err
	}
	r.SwitchSession(next)
	if snapshotID != "" && r.Snapshot != nil {
		_ = r.Snapshot.Remove(snapshotID)
	}
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
	name := strings.TrimPrefix(parts[0], "/")
	if item, ok := r.Commands[name]; ok {
		args := parts[1:]
		return true, r.promptCommand(ctx, item, args, out)
	}
	switch parts[0] {
	case "/help":
		r.Help(out)
	case "/status":
		r.Status(out)
	case "/usage":
		data := struct {
			Total   provider.UsageTotals            `json:"total"`
			ByModel map[string]provider.UsageTotals `json:"by_model"`
		}{Total: r.ProviderUsage(), ByModel: r.ProviderUsageByModel()}
		if err := json.NewEncoder(out).Encode(data); err != nil {
			return true, err
		}
	case "/history":
		r.History(out)
	case "/mcps":
		if err := json.NewEncoder(out).Encode(r.MCP()); err != nil {
			return true, err
		}
	case "/lsp":
		if err := json.NewEncoder(out).Encode(r.LSP()); err != nil {
			return true, err
		}
	case "/plugins":
		if err := json.NewEncoder(out).Encode(r.Plugins()); err != nil {
			return true, err
		}
	case "/skills":
		items, err := r.Skills()
		if err != nil {
			return true, err
		}
		for _, item := range items {
			fmt.Fprintf(out, "%s — %s\n", item.Name, item.Description)
		}
	case "/sessions", "/resume", "/continue":
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
	case "/variants", "/variant":
		if len(parts) < 2 {
			items, err := r.Variants(ctx)
			if err != nil {
				return true, err
			}
			for _, item := range items {
				fmt.Fprintln(out, item)
			}
			return true, nil
		}
		if err := r.SetVariant(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(out, "active variant:", r.VariantName())
	case "/agent", "/agents":
		if len(parts) < 2 {
			fmt.Fprintln(out, "active agent:", r.AgentName(), "(build, plan)")
			return true, nil
		}
		if err := r.SetAgent(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(out, "active agent:", r.AgentName())
	case "/model":
		if len(parts) < 2 {
			fmt.Fprintln(out, "active model:", r.ModelName())
			return true, nil
		}
		if err := r.SetModel(parts[1]); err != nil {
			return true, err
		}
		fmt.Fprintln(out, "active model:", r.ModelName())
	case "/clear", "/new":
		next, err := r.NewSession()
		if err != nil {
			return true, err
		}
		fmt.Fprintln(out, "New session:", next.ID)
	case "/fork":
		next, err := r.ForkSession()
		if err != nil {
			return true, err
		}
		fmt.Fprintln(out, "Fork session:", next.ID)
	case "/rename":
		if len(parts) < 2 {
			fmt.Fprintln(out, "Usage: /rename <title>")
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
	case "/exit", "/quit", "/q":
		return true, nil
	default:
		fmt.Fprintf(out, "Unknown command: %s\n", parts[0])
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

func (r *Runtime) PromptWithParts(ctx context.Context, text string, parts []schema.MessagePart, out io.Writer) error {
	return r.promptDelivery(ctx, text, session.DeliveryQueue, out, "", "", "", parts)
}

func (r *Runtime) promptCommand(ctx context.Context, item commandpkg.Info, args []string, out io.Writer) error {
	model, agentName, variant := r.ModelName(), r.AgentName(), item.Variant
	if item.Model != "" {
		model = item.Model
	}
	if item.Agent != "" {
		agentName = item.Agent
	}
	return r.promptDelivery(ctx, commandpkg.Expand(item.Template, args), session.DeliveryQueue, out, model, agentName, variant, nil)
}

func (r *Runtime) PromptDelivery(ctx context.Context, text string, delivery session.Delivery, out io.Writer) error {
	return r.promptDelivery(ctx, text, delivery, out, "", "", "", nil)
}

func (r *Runtime) promptDelivery(ctx context.Context, text string, delivery session.Delivery, out io.Writer, selectedModel, selectedAgent, selectedVariant string, parts []schema.MessagePart) error {
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
	user := session.Message{ID: session.NewMessageID(), Role: "user", Content: text, Parts: append([]schema.MessagePart(nil), parts...)}
	if r.Snapshot != nil {
		saved, err := r.Snapshot.Capture()
		if err != nil {
			return err
		}
		user.SnapshotID = saved.Manifest.ID
	}
	if r.Inputs == nil {
		r.Inputs = session.NewInputQueue()
	}
	input, err := r.AdmitInput(current.ID, text, delivery)
	if err != nil {
		if user.SnapshotID != "" && r.Snapshot != nil {
			_ = r.Snapshot.Remove(user.SnapshotID)
		}
		return err
	}
	if _, _, err := r.Inputs.PromoteByID(input.ID); err != nil {
		if user.SnapshotID != "" && r.Snapshot != nil {
			_ = r.Snapshot.Remove(user.SnapshotID)
		}
		return err
	}
	if err := r.Store.Append(current.ID, user); err != nil {
		if user.SnapshotID != "" && r.Snapshot != nil {
			_ = r.Snapshot.Remove(user.SnapshotID)
		}
		return err
	}
	current.Messages = append(current.Messages, user)
	current.Agent = r.AgentName()
	if selectedAgent == "" {
		selectedAgent = r.AgentName()
	}
	if selectedModel == "" {
		selectedModel = r.ModelName()
	}
	if selectedVariant == "" {
		selectedVariant = r.VariantName()
	}
	if next, stateErr := r.Store.SetRunState(current.ID, session.RunBusy, 0, ""); stateErr == nil {
		current.RunStatus = next.RunStatus
		current.RunAttempt = next.RunAttempt
		current.RunError = next.RunError
		current.RunStartedAt = next.RunStartedAt
		current.RunFinishedAt = next.RunFinishedAt
	}
	if r.Events != nil {
		_, _ = r.Events.Publish(ctx, schema.EventRunStarted, current.ID, map[string]any{"status": session.RunBusy})
	}
	options := runner.RunOptions{OnText: func(value string) {
		_, _ = io.WriteString(out, value)
		if r.Events != nil {
			_, _ = r.Events.Publish(ctx, schema.EventTextDelta, current.ID, map[string]any{"content": value})
		}
	}, OnToolStart: func(call schema.ToolCall) {
		if r.Events != nil {
			_, _ = r.Events.Publish(ctx, schema.EventToolStarted, current.ID, map[string]any{"name": call.Name, "call_id": call.ID})
		}
	}, OnDelta: func(delta protocol.StreamDelta) {
		if r.Events == nil {
			return
		}
		data := map[string]any{}
		if delta.Model != "" {
			data["model"] = delta.Model
		}
		if delta.Reasoning != "" {
			data["reasoning"] = delta.Reasoning
		}
		if delta.FinishReason != "" {
			data["finish_reason"] = delta.FinishReason
		}
		if delta.Usage != nil {
			data["usage"] = delta.Usage
		}
		if len(data) > 0 {
			_, _ = r.Events.Publish(ctx, schema.EventMessageMetadata, current.ID, data)
		}
	}, OnRetry: func(attempt int, retryErr error) {
		r.setRunState(ctx, current, session.RunRetrying, attempt, errorText(retryErr))
	}, OnTool: func(call schema.ToolCall, result string, err error) {
		if r.Events != nil {
			_, _ = r.Events.Publish(ctx, schema.EventToolFinished, current.ID, map[string]any{"name": call.Name, "error": errorText(err)})
		}
		if err != nil {
			_, _ = fmt.Fprintf(out, "\n[tool %s error: %v]\n", call.Name, err)
			return
		}
		_, _ = fmt.Fprintf(out, "\n[tool %s completed]\n", call.Name)
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
	runErr := r.Coordinator.Run(runCtx, current.ID, func(runCtx context.Context) error {
		r.Runner.Agent = selectedAgent
		r.Runner.Variant = selectedVariant
		return r.Runner.RunWithOptions(runCtx, current, selectedModel, r.SystemPromptFor(selectedAgent), options)
	})
	if runErr != nil {
		state := session.RunFailed
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			state = session.RunInterrupted
		}
		r.setRunState(ctx, current, state, 0, runErr.Error())
		return runErr
	}
	r.setRunState(ctx, current, session.RunCompleted, 0, "")
	_, err = io.WriteString(out, "\n")
	return err
}

func (r *Runtime) setRunState(ctx context.Context, current *session.Session, status string, attempt int, runErr string) {
	if next, err := r.Store.SetRunState(current.ID, status, attempt, runErr); err == nil {
		current.RunStatus, current.RunAttempt, current.RunError = next.RunStatus, next.RunAttempt, next.RunError
		current.RunStartedAt, current.RunFinishedAt = next.RunStartedAt, next.RunFinishedAt
	}
	if r.Events == nil {
		return
	}
	typ := schema.EventRunCompleted
	if status == session.RunRetrying {
		typ = schema.EventRunRetrying
	}
	if status == session.RunFailed {
		typ = schema.EventRunFailed
	}
	if status == session.RunInterrupted {
		typ = schema.EventRunInterrupted
	}
	_, _ = r.Events.Publish(ctx, typ, current.ID, map[string]any{"status": status, "attempt": attempt, "error": runErr})
}
func (r *Runtime) Help(out io.Writer) {
	fmt.Fprintln(out, "OpenCode commands:")
	fmt.Fprintln(out, "  /help                  show help")
	fmt.Fprintln(out, "  /status                show project and session status")
	fmt.Fprintln(out, "  /usage                 show provider usage")
	fmt.Fprintln(out, "  /models                list available models")
	fmt.Fprintln(out, "  /model <id>            select a model")
	fmt.Fprintln(out, "  /variants              list model variants")
	fmt.Fprintln(out, "  /variant <name>        select a model variant")
	fmt.Fprintln(out, "  /agents                list or select an agent")
	fmt.Fprintln(out, "  /agent <name>          select an agent")
	fmt.Fprintln(out, "  /sessions              switch session")
	fmt.Fprintln(out, "  /resume, /continue     switch session")
	fmt.Fprintln(out, "  /new, /clear           create a new session")
	fmt.Fprintln(out, "  /fork                  fork the current session")
	fmt.Fprintln(out, "  /rename <title>        rename the current session")
	fmt.Fprintln(out, "  /export [md|json]      export the current session")
	fmt.Fprintln(out, "  /history               show session history")
	fmt.Fprintln(out, "  /skills                list discovered skills")
	fmt.Fprintln(out, "  /mcps                  show MCP integration status")
	fmt.Fprintln(out, "  /lsp                   show LSP integration status")
	fmt.Fprintln(out, "  /plugins               show plugin integration status")
	fmt.Fprintln(out, "  /exit, /quit, /q       exit")
}
func (r *Runtime) Status(out io.Writer) {
	fmt.Fprintf(out, "project: %s\nmodel: %s\nsession: %s\ntitle: %s\n", r.Root, r.Config.Model, r.Session.ID, r.Session.Title)
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
