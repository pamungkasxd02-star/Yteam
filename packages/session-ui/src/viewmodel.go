package sessionui

import (
	"strings"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

// ViewMessage represents a formatted message view ready for presentation.
type ViewMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionViewModel packages a session into an immutable view model for TUI/Web.
type SessionViewModel struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Directory   string        `json:"directory"`
	MessageList []ViewMessage `json:"messages"`
}

func FromSession(s session.Session) SessionViewModel {
	vm := SessionViewModel{
		ID:        s.ID,
		Title:     s.Title,
		Directory: s.Directory,
	}
	if vm.Title == "" {
		vm.Title = "Untitled"
	}

	for _, m := range s.Messages {
		author := strings.ToUpper(m.Role[:1]) + m.Role[1:]
		if m.Role == "user" {
			author = "You"
		} else if m.Role == "assistant" {
			author = "Agent"
		}
		ts, _ := time.Parse(time.RFC3339, m.CreatedAt)
		vm.MessageList = append(vm.MessageList, ViewMessage{
			ID:        m.ID,
			Role:      m.Role,
			Author:    author,
			Content:   m.Content,
			Timestamp: ts,
		})
	}
	return vm
}
