package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Handler represents a standalone function execution callback.
type Handler func(ctx context.Context, args map[string]any) (any, error)

// Registry manages serverless functions/tools callable by LLM agents.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
	}
}

func (r *Registry) Register(name string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

func (r *Registry) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	r.mu.RLock()
	h, ok := r.handlers[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("function %q not found", name)
	}
	return h(ctx, args)
}

func (r *Registry) CallJSON(ctx context.Context, name string, rawJSON []byte) (string, error) {
	var args map[string]any
	if len(rawJSON) > 0 {
		if err := json.Unmarshal(rawJSON, &args); err != nil {
			return "", errors.New("invalid JSON arguments")
		}
	}
	res, err := r.Call(ctx, name, args)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
