package schema

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role         Role          `json:"role"`
	Content      string        `json:"content,omitempty"`
	Reasoning    string        `json:"reasoning,omitempty"`
	Model        string        `json:"model,omitempty"`
	FinishReason string        `json:"finish_reason,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
	Parts        []MessagePart `json:"parts,omitempty"`
	Name         string        `json:"name,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
}

type MessagePart struct {
	Type       string    `json:"type"`
	Text       string    `json:"text,omitempty"`
	ToolCall   *ToolCall `json:"tool_call,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
	State      string    `json:"state,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type Usage struct {
	PromptTokens            int           `json:"prompt_tokens,omitempty"`
	CompletionTokens        int           `json:"completion_tokens,omitempty"`
	TotalTokens             int           `json:"total_tokens,omitempty"`
	PromptTokensDetails     *TokenDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *TokenDetails `json:"completion_tokens_details,omitempty"`
}

type TokenDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (call ToolCall) MarshalJSON() ([]byte, error) {
	type function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	return json.Marshal(struct {
		ID       string   `json:"id"`
		Type     string   `json:"type"`
		Function function `json:"function"`
	}{
		ID: call.ID, Type: call.Type,
		Function: function{Name: call.Name, Arguments: call.Arguments},
	})
}

func (call *ToolCall) UnmarshalJSON(data []byte) error {
	type function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	var wire struct {
		ID        string   `json:"id"`
		Type      string   `json:"type"`
		Name      string   `json:"name"`
		Arguments string   `json:"arguments"`
		Function  function `json:"function"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	call.ID, call.Type = wire.ID, wire.Type
	call.Name, call.Arguments = wire.Name, wire.Arguments
	if call.Name == "" {
		call.Name = wire.Function.Name
	}
	if call.Arguments == "" {
		call.Arguments = wire.Function.Arguments
	}
	return nil
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

type FunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}
