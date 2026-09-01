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
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
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
