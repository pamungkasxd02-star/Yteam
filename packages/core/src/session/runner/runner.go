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
	OnText   func(string)
	OnTool   func(schema.ToolCall, string, error)
}

func (r *Runner) Run(ctx context.Context, sess *session.Session, model, system string) error {
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
			messages = append(messages, schema.Message{Role: schema.Role(item.Role), Content: item.Content, Name: item.Name, ToolCallID: item.ToolCallID, ToolCalls: item.ToolCalls})
		}
		var text strings.Builder
		var calls []schema.ToolCall
		err := r.Provider.Complete(ctx, protocol.ChatRequest{Model: model, Messages: messages, Tools: r.toolDefinitions()}, func(delta protocol.StreamDelta) error {
			if delta.Content != "" {
				text.WriteString(delta.Content)
				if r.OnText != nil {
					r.OnText(delta.Content)
				}
			}
			if len(delta.ToolCalls) > 0 {
				calls = append(calls, delta.ToolCalls...)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if text.Len() > 0 || len(calls) > 0 {
			assistant := session.Message{Role: "assistant", Content: text.String(), ToolCalls: calls}
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
			if r.OnTool != nil {
				r.OnTool(call, results[index], errs[index])
			}
			content := results[index]
			if errs[index] != nil {
				content = "tool error: " + errs[index].Error()
			}
			if err := r.appendMessage(sess, session.Message{Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: content}); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("agent stopped after %d steps", max)
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
