package tui

import (
	"fmt"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

// TranscriptReducer is the small projection layer used by the terminal UI.
// Durable session storage remains the source of truth; this reducer only
// applies live events between durable reloads.
type TranscriptReducer struct {
	Messages []LiveMessage
	Status   string
}

type LiveMessage struct {
	Role      string
	Content   string
	ToolName  string
	ToolState string
}

func NewTranscriptReducer() *TranscriptReducer { return &TranscriptReducer{Status: "idle"} }

func (r *TranscriptReducer) Hydrate(messages []session.Message) {
	r.Messages = r.Messages[:0]
	for _, message := range messages {
		r.Messages = append(r.Messages, LiveMessage{Role: message.Role, Content: message.Content})
	}
}

func (r *TranscriptReducer) Apply(event schema.Event) {
	switch event.Type {
	case schema.EventPromptAdmitted:
		r.Messages = append(r.Messages, LiveMessage{Role: "user", Content: stringValue(event.Data["content"])})
		r.Status = "running"
	case schema.EventTextDelta:
		text := stringValue(event.Data["content"])
		if text == "" {
			return
		}
		if len(r.Messages) == 0 || r.Messages[len(r.Messages)-1].Role != "assistant" {
			r.Messages = append(r.Messages, LiveMessage{Role: "assistant"})
		}
		r.Messages[len(r.Messages)-1].Content += text
	case schema.EventToolStarted:
		r.Messages = append(r.Messages, LiveMessage{Role: "tool", ToolName: stringValue(event.Data["name"]), ToolState: "running"})
		r.Status = "tool"
	case schema.EventToolFinished:
		name := stringValue(event.Data["name"])
		for index := len(r.Messages) - 1; index >= 0; index-- {
			if r.Messages[index].Role == "tool" && (name == "" || r.Messages[index].ToolName == name) {
				r.Messages[index].ToolState = "error"
				if stringValue(event.Data["error"]) == "" {
					r.Messages[index].ToolState = "done"
				}
				break
			}
		}
		r.Status = "running"
	case schema.EventPermissionAsked:
		r.Status = "waiting for permission"
	case schema.EventPermissionReply:
		r.Status = "running"
	}
}

func (r *TranscriptReducer) String() string {
	var out strings.Builder
	for _, item := range r.Messages {
		if item.Role == "tool" {
			fmt.Fprintf(&out, "tool %s (%s): %s\n", item.ToolName, item.ToolState, item.Content)
			continue
		}
		fmt.Fprintf(&out, "%s: %s\n", item.Role, item.Content)
	}
	return out.String()
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
