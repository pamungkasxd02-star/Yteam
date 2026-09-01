package protocol

import "github.com/pamungkasxd02-star/Yteam/packages/schema/src"

type ChatRequest struct {
	Model    string           `json:"model"`
	Messages []schema.Message `json:"messages"`
	Stream   bool             `json:"stream"`
}

type StreamDelta struct {
	Content string
}

type Model struct {
	ID string `json:"id"`
}
