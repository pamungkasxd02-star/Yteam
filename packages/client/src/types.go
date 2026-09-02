package client

import (
	"fmt"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Status struct {
	Project string          `json:"project"`
	Model   string          `json:"model"`
	Session session.Session `json:"session"`
}

type Agent struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mode        string `json:"mode"`
}

type AgentState struct {
	Current string  `json:"current"`
	Agents  []Agent `json:"agents"`
}

type Selection struct {
	Current string `json:"current"`
}

type ProviderUsage struct {
	Total   provider.UsageTotals            `json:"total"`
	ByModel map[string]provider.UsageTotals `json:"by_model"`
}

type IntegrationStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Tools  int    `json:"tools"`
	Error  string `json:"error,omitempty"`
}

type SessionContext struct {
	SessionID string            `json:"session_id"`
	Messages  []session.Message `json:"messages"`
}

type SessionHistory = SessionContext

type QuestionState struct {
	Requests []schema.QuestionRequest `json:"requests"`
}

type APIError struct {
	Status    int
	Message   string
	RequestID string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	message := e.Message
	if message == "" {
		message = fmt.Sprintf("server returned HTTP %d", e.Status)
	}
	if e.RequestID != "" {
		message += " (request " + e.RequestID + ")"
	}
	return message
}
