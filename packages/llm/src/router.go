package llm

import (
	"context"
	"errors"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

// ProviderType represents supported LLM backends.
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderGemini    ProviderType = "gemini"
	ProviderOllama    ProviderType = "ollama"
	ProviderOpenRouter ProviderType = "openrouter"
)

// StreamDelta is an alias to protocol.StreamDelta for package independence.
type StreamDelta = protocol.StreamDelta

// Adapter is the common interface implemented by all LLM backends.
type Adapter interface {
	Name() string
	Type() ProviderType
	Models(ctx context.Context) ([]protocol.Model, error)
	Complete(ctx context.Context, req protocol.ChatRequest, emit func(StreamDelta) error) error
}

// Router dispatches chat requests to the appropriate provider adapter based on model prefix.
type Router struct {
	adapters map[ProviderType]Adapter
	fallback Adapter
}

func NewRouter(fallback Adapter) *Router {
	return &Router{
		adapters: make(map[ProviderType]Adapter),
		fallback: fallback,
	}
}

func (r *Router) Register(pType ProviderType, adapter Adapter) {
	r.adapters[pType] = adapter
}

func (r *Router) Select(modelName string) Adapter {
	modelLower := strings.ToLower(modelName)
	if strings.HasPrefix(modelLower, "anthropic/") || strings.HasPrefix(modelLower, "claude-") {
		if a, ok := r.adapters[ProviderAnthropic]; ok {
			return a
		}
	}
	if strings.HasPrefix(modelLower, "gemini/") || strings.HasPrefix(modelLower, "google/") || strings.HasPrefix(modelLower, "gemini-") {
		if a, ok := r.adapters[ProviderGemini]; ok {
			return a
		}
	}
	if strings.HasPrefix(modelLower, "ollama/") {
		if a, ok := r.adapters[ProviderOllama]; ok {
			return a
		}
	}
	if strings.HasPrefix(modelLower, "openrouter/") {
		if a, ok := r.adapters[ProviderOpenRouter]; ok {
			return a
		}
	}
	if a, ok := r.adapters[ProviderOpenAI]; ok {
		return a
	}
	return r.fallback
}

func (r *Router) Complete(ctx context.Context, req protocol.ChatRequest, emit func(StreamDelta) error) error {
	adapter := r.Select(req.Model)
	if adapter == nil {
		return errors.New("no suitable LLM adapter available")
	}
	return adapter.Complete(ctx, req, emit)
}

func (r *Router) Models(ctx context.Context) ([]protocol.Model, error) {
	var all []protocol.Model
	seen := make(map[string]bool)
	for _, a := range r.adapters {
		models, err := a.Models(ctx)
		if err == nil {
			for _, m := range models {
				if !seen[m.ID] {
					seen[m.ID] = true
					all = append(all, m)
				}
			}
		}
	}
	if len(all) == 0 && r.fallback != nil {
		return r.fallback.Models(ctx)
	}
	return all, nil
}
