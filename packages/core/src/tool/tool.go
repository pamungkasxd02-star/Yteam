package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Context struct {
	SessionID string
	Agent     string
	CallID    string
	Root      string
}
type Definition interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Execute(context.Context, Context, json.RawMessage) (string, error)
}

// ExternalCaller lets MCP/plugin adapters participate in the same permission
// and tool-call path as built-in tools.
type ExternalCaller interface {
	CallTool(context.Context, string, map[string]any) (string, error)
}

type Questioner interface {
	AskQuestion(string, []schema.QuestionInfo, *schema.QuestionToolRef) (schema.QuestionRequest, error)
	AwaitQuestion(context.Context, string) ([]schema.QuestionAnswer, error)
}

type Question struct{ Manager Questioner }

func (Question) Name() string        { return "question" }
func (Question) Description() string { return "Ask the user a question and wait for an answer" }
func (Question) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"questions": map[string]any{"type": "array"}}, "required": []string{"questions"}, "additionalProperties": false}
}
func (q Question) Execute(ctx context.Context, tc Context, raw json.RawMessage) (string, error) {
	if q.Manager == nil {
		return "", errors.New("question manager is not configured")
	}
	var input struct {
		Questions []schema.QuestionInfo `json:"questions"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || len(input.Questions) == 0 {
		return "", errors.New("question requires questions")
	}
	request, err := q.Manager.AskQuestion(tc.SessionID, input.Questions, &schema.QuestionToolRef{MessageID: tc.CallID, CallID: tc.CallID})
	if err != nil {
		return "", err
	}
	answers, err := q.Manager.AwaitQuestion(ctx, request.ID)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(answers)
	return string(data), nil
}

type ExternalTool struct {
	Caller          ExternalCaller
	ToolName        string
	RemoteName      string
	ToolDescription string
	ToolParameters  map[string]any
}

func (t ExternalTool) Name() string               { return t.ToolName }
func (t ExternalTool) Description() string        { return t.ToolDescription }
func (t ExternalTool) Parameters() map[string]any { return t.ToolParameters }
func (t ExternalTool) Execute(ctx context.Context, _ Context, raw json.RawMessage) (string, error) {
	if t.Caller == nil {
		return "", errors.New("external tool caller is nil")
	}
	var arguments map[string]any
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return "", err
	}
	remoteName := t.RemoteName
	if remoteName == "" {
		remoteName = t.ToolName
	}
	return t.Caller.CallTool(ctx, remoteName, arguments)
}

type Registry struct {
	items       map[string]Definition
	permissions *permission.Engine
	approve     func(permission.Request) permission.Reply
}

func ReadOnly(p *permission.Engine) *Registry {
	registry := NewRegistry(p)
	registry.Add(ReadFile{})
	registry.Add(ListDir{})
	registry.Add(GlobFiles{})
	registry.Add(GrepFiles{})
	return registry
}

func Builtins(p *permission.Engine) *Registry {
	registry := ReadOnly(p)
	registry.Add(WriteFile{})
	registry.Add(EditFile{})
	registry.Add(Bash{})
	registry.Add(NewWebFetch())
	registry.Add(NewWebSearch())
	return registry
}

func NewRegistry(p *permission.Engine) *Registry {
	return &Registry{items: map[string]Definition{}, permissions: p}
}
func (r *Registry) SetApproval(approve func(permission.Request) permission.Reply) {
	r.approve = approve
}

func (r *Registry) Approval() func(permission.Request) permission.Reply {
	return r.approve
}
func (r *Registry) Add(definition Definition) { r.items[definition.Name()] = definition }
func (r *Registry) Definitions() []schema.ToolDefinition {
	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]schema.ToolDefinition, 0, len(names))
	for _, name := range names {
		item := r.items[name]
		result = append(result, schema.ToolDefinition{Type: "function", Function: schema.FunctionSpec{Name: name, Description: item.Description(), Parameters: item.Parameters()}})
	}
	return result
}
func (r *Registry) Execute(ctx context.Context, call schema.ToolCall, tc Context) (string, error) {
	item, ok := r.items[call.Name]
	if !ok {
		return "", fmt.Errorf("tool %q not found", call.Name)
	}
	if r.permissions != nil {
		request, err := r.permissions.Assert(tc.SessionID, call.Name, call.Name)
		if err != nil {
			return "", err
		}
		if request.ID != "" {
			if r.approve != nil {
				if err := r.permissions.Reply(request.ID, r.approve(request)); err != nil {
					return "", err
				}
			} else if err := r.permissions.Await(ctx, request.ID); err != nil {
				return "", err
			}
		}
	}
	return item.Execute(ctx, tc, json.RawMessage(call.Arguments))
}

type ReadFile struct{}

func (ReadFile) Name() string        { return "read" }
func (ReadFile) Description() string { return "Read a UTF-8 text file inside the project" }
func (ReadFile) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}, "additionalProperties": false}
}
func (ReadFile) Execute(_ context.Context, tc Context, raw json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Path == "" {
		return "", errors.New("read requires path")
	}
	path, err := safePath(tc.Root, input.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > 1024*1024 {
		return string(data[:1024*1024]), errors.New("file output truncated at 1 MiB")
	}
	return string(data), nil
}

type ListDir struct{}

func (ListDir) Name() string        { return "list" }
func (ListDir) Description() string { return "List files and directories inside the project" }
func (ListDir) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}
}
func (ListDir) Execute(_ context.Context, tc Context, raw json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(raw, &input)
	path, err := safePath(tc.Root, input.Path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var result []string
	for _, entry := range entries {
		suffix := ""
		if entry.IsDir() {
			suffix = string(filepath.Separator)
		}
		result = append(result, entry.Name()+suffix)
	}
	return strings.Join(result, "\n"), nil
}

type GlobFiles struct{}

func (GlobFiles) Name() string        { return "glob" }
func (GlobFiles) Description() string { return "Find files matching a pattern inside the project" }
func (GlobFiles) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string"}}, "required": []string{"pattern"}, "additionalProperties": false}
}
func (GlobFiles) Execute(_ context.Context, tc Context, raw json.RawMessage) (string, error) {
	var input struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Pattern == "" {
		return "", errors.New("glob requires pattern")
	}
	pattern := filepath.Join(tc.Root, input.Pattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if safe, err := safePath(tc.Root, strings.TrimPrefix(match, tc.Root+string(filepath.Separator))); err == nil {
			relative, _ := filepath.Rel(tc.Root, safe)
			result = append(result, relative)
		}
	}
	sort.Strings(result)
	return strings.Join(result, "\n"), nil
}

type GrepFiles struct{}

func (GrepFiles) Name() string        { return "grep" }
func (GrepFiles) Description() string { return "Search text files inside the project" }
func (GrepFiles) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"pattern": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}}, "required": []string{"pattern"}, "additionalProperties": false}
}
func (GrepFiles) Execute(_ context.Context, tc Context, raw json.RawMessage) (string, error) {
	var input struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Pattern == "" {
		return "", errors.New("grep requires pattern")
	}
	root, err := safePath(tc.Root, input.Path)
	if err != nil {
		return "", err
	}
	needle := []byte(input.Pattern)
	var result []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || info.Size() > 1024*1024 {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if !bytes.Contains(data, needle) {
			return nil
		}
		relative, _ := filepath.Rel(tc.Root, path)
		result = append(result, relative)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(result)
	return strings.Join(result, "\n"), nil
}

type WriteFile struct{}

func (WriteFile) Name() string        { return "write" }
func (WriteFile) Description() string { return "Write a text file inside the project" }
func (WriteFile) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"path", "content"}, "additionalProperties": false}
}
func (WriteFile) Execute(_ context.Context, tc Context, raw json.RawMessage) (string, error) {
	var input struct{ Path, Content string }
	if err := json.Unmarshal(raw, &input); err != nil || input.Path == "" {
		return "", errors.New("write requires path and content")
	}
	path, err := safePath(tc.Root, input.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(input.Content), 0o600); err != nil {
		return "", err
	}
	return "wrote " + input.Path, nil
}

type EditFile struct{}

func (EditFile) Name() string        { return "edit" }
func (EditFile) Description() string { return "Replace one exact text fragment in a project file" }
func (EditFile) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "old": map[string]any{"type": "string"}, "new": map[string]any{"type": "string"}}, "required": []string{"path", "old", "new"}, "additionalProperties": false}
}
func (EditFile) Execute(_ context.Context, tc Context, raw json.RawMessage) (string, error) {
	var input struct{ Path, Old, New string }
	if err := json.Unmarshal(raw, &input); err != nil || input.Path == "" || input.Old == "" {
		return "", errors.New("edit requires path, old, and new")
	}
	path, err := safePath(tc.Root, input.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	before := string(data)
	if strings.Count(before, input.Old) != 1 {
		return "", errors.New("edit requires exactly one matching fragment")
	}
	after := strings.Replace(before, input.Old, input.New, 1)
	if err := os.WriteFile(path, []byte(after), 0o600); err != nil {
		return "", err
	}
	return "edited " + input.Path, nil
}

type Bash struct{}

func (Bash) Name() string { return "bash" }
func (Bash) Description() string {
	return "Run a shell command in the project after permission approval"
}
func (Bash) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}, "timeout_ms": map[string]any{"type": "integer"}}, "required": []string{"command"}, "additionalProperties": false}
}
func (Bash) Execute(ctx context.Context, tc Context, raw json.RawMessage) (string, error) {
	var input struct {
		Command   string `json:"command"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.Command) == "" {
		return "", errors.New("bash requires command")
	}
	if input.TimeoutMS <= 0 || input.TimeoutMS > 120000 {
		input.TimeoutMS = 30000
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(input.TimeoutMS)*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(runCtx, shellName(), shellArg(), input.Command)
	cmd.Dir = tc.Root
	output, err := cmd.CombinedOutput()
	text := string(output)
	if len(text) > 1024*1024 {
		text = text[:1024*1024] + "\n[output truncated]"
	}
	if err != nil {
		return text, fmt.Errorf("command failed: %w", err)
	}
	return text, nil
}
func shellName() string {
	if goruntime.GOOS == "windows" {
		if comSpec := os.Getenv("ComSpec"); comSpec != "" {
			return comSpec
		}
		return "cmd.exe"
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/sh"
}
func shellArg() string {
	if goruntime.GOOS == "windows" {
		return "/C"
	}
	return "-c"
}

func safePath(root, input string) (string, error) {
	if root == "" {
		return "", errors.New("project root is empty")
	}
	root, _ = filepath.Abs(root)
	path, err := filepath.Abs(filepath.Join(root, input))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path is outside project root")
	}
	return path, nil
}
