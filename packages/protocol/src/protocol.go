package protocol

import "github.com/pamungkasxd02-star/Yteam/packages/schema/src"

type ChatRequest struct {
	Model       string                  `json:"model"`
	Messages    []schema.Message        `json:"messages"`
	Tools       []schema.ToolDefinition `json:"tools,omitempty"`
	ToolChoice  any                     `json:"tool_choice,omitempty"`
	Temperature *float64                `json:"temperature,omitempty"`
	Stream      bool                    `json:"stream"`
}

type StreamDelta struct {
	Content   string
	Reasoning string
	ToolCalls []schema.ToolCall
}

type Model struct {
	ID string `json:"id"`
}
