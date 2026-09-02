package protocol

import "github.com/pamungkasxd02-star/Yteam/packages/schema/src"

type ChatRequest struct {
	Model       string                  `json:"model"`
	Variant     string                  `json:"variant,omitempty"`
	Options     map[string]any          `json:"options,omitempty"`
	Messages    []schema.Message        `json:"messages"`
	Tools       []schema.ToolDefinition `json:"tools,omitempty"`
	ToolChoice  any                     `json:"tool_choice,omitempty"`
	Temperature *float64                `json:"temperature,omitempty"`
	Stream      bool                    `json:"stream"`
}

type StreamDelta struct {
	Content      string
	Reasoning    string
	ToolCalls    []schema.ToolCall
	Usage        *Usage
	Model        string
	FinishReason string
}

type Model struct {
	ID                 string                  `json:"id"`
	Object             string                  `json:"object,omitempty"`
	Created            int64                   `json:"created,omitempty"`
	OwnedBy            string                  `json:"owned_by,omitempty"`
	Name               string                  `json:"name,omitempty"`
	Provider           string                  `json:"provider,omitempty"`
	ContextWindow      int                     `json:"context_window,omitempty"`
	MaxOutputTokens    int                     `json:"max_output_tokens,omitempty"`
	InputCostPerToken  float64                 `json:"input_cost_per_token,omitempty"`
	OutputCostPerToken float64                 `json:"output_cost_per_token,omitempty"`
	Variants           map[string]ModelVariant `json:"variants,omitempty"`
}

type ModelVariant struct {
	Temperature *float64       `json:"temperature,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
}

type Usage = schema.Usage
type TokenDetails = schema.TokenDetails
