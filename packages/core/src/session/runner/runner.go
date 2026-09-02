package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/tool"
	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

// Runner preserves OpenCode's important turn invariant: one provider stream is
// one logical turn. Tool calls are settled locally, then their results become
// tool messages for the next provider turn.
type Runner struct {
	Provider *provider.Client
	Tools    *tool.Registry
	Store    *session.Store
	MaxSteps int
}

func (r *Runner) Run(ctx context.Context, sess *session.Session, model, system string) error {
	return r.RunWithOptions(ctx, sess, model, system, RunOptions{})
}

type RunOptions struct {
	OnText      func(string)
	OnDelta     func(protocol.StreamDelta)
	OnRetry     func(int, error)
	OnToolStart func(schema.ToolCall)
	OnTool      func(schema.ToolCall, string, error)
}

func (r *Runner) RunWithOptions(ctx context.Context, sess *session.Session, model, system string, options RunOptions) error {
	max := r.MaxSteps
	if max <= 0 {
		max = 8
	}
	for step := 0; step < max; step++ {
		messages := make([]schema.Message, 0, len(sess.Messages)+1)
		if system != "" {
			messages = append(messages, schema.Message{Role: schema.RoleSystem, Content: system})
		}
		for _, item := range sess.Messages {
			messages = append(messages, schema.Message{Role: schema.Role(item.Role), Content: item.Content, Reasoning: item.Reasoning, Model: item.Model, FinishReason: item.FinishReason, Usage: item.Usage, Parts: item.Parts, Name: item.Name, ToolCallID: item.ToolCallID, ToolCalls: item.ToolCalls})
		}
		var text strings.Builder
		var calls []schema.ToolCall
		var reasoning string
		var usage *protocol.Usage
		responseModel := ""
		finishReason := ""
		err := r.Provider.CompleteRetryWithStatus(ctx, protocol.ChatRequest{Model: model, Messages: messages, Tools: r.toolDefinitions()}, func(delta protocol.StreamDelta) error {
			if delta.Content != "" {
				text.WriteString(delta.Content)
				if options.OnText != nil {
					options.OnText(delta.Content)
				}
			}
			if len(delta.ToolCalls) > 0 {
				calls = append(calls, delta.ToolCalls...)
			}
			if delta.Reasoning != "" {
				reasoning += delta.Reasoning
			}
			if delta.Usage != nil {
				usage = delta.Usage
			}
			if delta.Model != "" {
				responseModel = delta.Model
			}
			if delta.FinishReason != "" {
				finishReason = delta.FinishReason
			}
			if options.OnDelta != nil {
				options.OnDelta(delta)
			}
			return nil
		}, options.OnRetry)
		if err != nil {
			return err
		}
		if text.Len() > 0 || reasoning != "" || usage != nil || responseModel != "" || finishReason != "" || len(calls) > 0 {
			parts := []schema.MessagePart{}
			if text.Len() > 0 {
				parts = append(parts, schema.MessagePart{Type: "text", Text: text.String()})
			}
			if reasoning != "" {
				parts = append(parts, schema.MessagePart{Type: "reasoning", Text: reasoning})
			}
			for _, call := range calls {
				toolCall := call
				parts = append(parts, schema.MessagePart{Type: "tool-call", ToolCall: &toolCall, ToolCallID: call.ID, State: "pending"})
			}
			assistant := session.Message{ID: session.NewMessageID(), Role: "assistant", Content: text.String(), Reasoning: reasoning, Model: responseModel, FinishReason: finishReason, Usage: usage, Parts: parts, ToolCalls: calls}
			if err := r.appendMessage(sess, assistant); err != nil {
				return err
			}
		}
		if len(calls) == 0 {
			return nil
		}
		results := make([]string, len(calls))
		errs := make([]error, len(calls))
		var group sync.WaitGroup
		for index, call := range calls {
			if options.OnToolStart != nil {
				options.OnToolStart(call)
			}
			group.Add(1)
			go func(index int, call schema.ToolCall) {
				defer group.Done()
				if r.Tools == nil {
					errs[index] = fmt.Errorf("tool registry is not configured")
					return
				}
				results[index], errs[index] = r.Tools.Execute(ctx, call, tool.Context{SessionID: sess.ID, Agent: "build", CallID: call.ID, Root: sess.Directory})
			}(index, call)
		}
		group.Wait()
		for index, call := range calls {
			if options.OnTool != nil {
				options.OnTool(call, results[index], errs[index])
			}
			content := results[index]
			if errs[index] != nil {
				content = "tool error: " + errs[index].Error()
			}
			if err := r.appendMessage(sess, session.Message{ID: session.NewMessageID(), Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: content, Parts: []schema.MessagePart{{Type: "tool-result", ToolCallID: call.ID, Text: content, State: toolState(errs[index])}}}); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("agent stopped after %d steps", max)
}

func toolState(err error) string {
	if err != nil {
		return "error"
	}
	return "done"
}

func (r *Runner) toolDefinitions() []schema.ToolDefinition {
	if r.Tools == nil {
		return nil
	}
	return r.Tools.Definitions()
}
func (r *Runner) appendMessage(sess *session.Session, message session.Message) error {
	if sess.Messages == nil {
		sess.Messages = []session.Message{}
	}
	sess.Messages = append(sess.Messages, message)
	if r.Store != nil {
		return r.Store.Append(sess.ID, message)
	}
	return nil
}
