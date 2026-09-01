package schema

import "time"

type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Data      map[string]any `json:"data"`
	Aggregate string         `json:"aggregate,omitempty"`
	Sequence  uint64         `json:"sequence,omitempty"`
	Version   int            `json:"version,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

const (
	EventSessionCreated  = "session.created"
	EventPromptAdmitted  = "session.prompt.admitted"
	EventPrompted        = "session.prompted"
	EventTextDelta       = "message.text.delta"
	EventToolStarted     = "tool.started"
	EventToolFinished    = "tool.finished"
	EventPermissionAsked = "permission.asked"
	EventPermissionReply = "permission.replied"
)
