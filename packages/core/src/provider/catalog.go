package provider

import (
	"context"
	"sort"
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

// Catalog is the provider model registry. It separates explicit discovery
// from cached lookup so response accounting never causes an extra network
// request while a completion is being processed.
type Catalog struct {
	client *Client
	mu     sync.RWMutex
	items  []protocol.Model
}

func NewCatalog(client *Client) *Catalog { return &Catalog{client: client} }

func (c *Catalog) Refresh(ctx context.Context) ([]protocol.Model, error) {
	items, err := c.client.Models(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	c.mu.Lock()
	c.items = append([]protocol.Model(nil), items...)
	c.mu.Unlock()
	return append([]protocol.Model(nil), items...), nil
}

func (c *Catalog) List(ctx context.Context) ([]protocol.Model, error) {
	c.mu.RLock()
	items := append([]protocol.Model(nil), c.items...)
	c.mu.RUnlock()
	if len(items) > 0 {
		return items, nil
	}
	return c.Refresh(ctx)
}

func (c *Catalog) Find(ctx context.Context, id string) (protocol.Model, error) {
	if item, ok := c.FindCached(id); ok {
		return item, nil
	}
	items, err := c.List(ctx)
	if err != nil {
		return protocol.Model{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return protocol.Model{ID: id}, nil
}

func (c *Catalog) FindCached(id string) (protocol.Model, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, item := range c.items {
		if item.ID == id {
			return item, true
		}
	}
	return protocol.Model{}, false
}

func (c *Catalog) ApplyVariant(input protocol.ChatRequest) protocol.ChatRequest {
	if input.Variant == "" {
		return input
	}
	model, ok := c.FindCached(input.Model)
	if !ok {
		return input
	}
	variant, ok := model.Variants[input.Variant]
	if !ok {
		return input
	}
	if variant.Temperature != nil && input.Temperature == nil {
		input.Temperature = variant.Temperature
	}
	if len(variant.Options) > 0 {
		if input.Options == nil {
			input.Options = map[string]any{}
		}
		for key, value := range variant.Options {
			if _, exists := input.Options[key]; !exists {
				input.Options[key] = value
			}
		}
	}
	return input
}
