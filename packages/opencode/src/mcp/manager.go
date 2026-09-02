package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
)

type Status struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Tools  int    `json:"tools"`
	Error  string `json:"error,omitempty"`
}

type Manager struct {
	mu      sync.RWMutex
	clients map[string]closer
	status  map[string]Status
}

type closer interface{ Close() error }

func NewManager() *Manager {
	return &Manager{clients: map[string]closer{}, status: map[string]Status{}}
}

func (m *Manager) Connect(ctx context.Context, app *runtime.Runtime, name string, cfg Config) error {
	client, err := Start(ctx, cfg)
	if err != nil {
		m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	if err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	tools, err := client.Tools(ctx)
	if err != nil {
		_ = client.Close()
		m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	for _, item := range tools {
		if err := app.AddExternalToolNamed(client, ToolName(name, item.Name), item.Name, item.Description, NormalizeInputSchema(item.InputSchema)); err != nil {
			_ = client.Close()
			m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
			return err
		}
	}
	m.mu.Lock()
	m.clients[name] = client
	m.status[name] = Status{Name: name, Status: "connected", Tools: len(tools)}
	m.mu.Unlock()
	return nil
}

func (m *Manager) ConnectRemote(ctx context.Context, app *runtime.Runtime, name string, cfg RemoteConfig) error {
	remote, err := NewRemote(cfg)
	if err != nil {
		m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	if err := remote.Initialize(ctx); err != nil {
		m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	tools, err := remote.AllTools(ctx)
	if err != nil {
		m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	for _, item := range tools {
		if err := app.AddExternalToolNamed(remote, ToolName(name, item.Name), item.Name, item.Description, NormalizeInputSchema(item.InputSchema)); err != nil {
			m.setStatus(Status{Name: name, Status: "failed", Error: err.Error()})
			return err
		}
	}
	m.mu.Lock()
	m.clients[name] = remote
	m.status[name] = Status{Name: name, Status: "connected", Tools: len(tools)}
	m.mu.Unlock()
	return nil
}

func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	client := m.clients[name]
	delete(m.clients, name)
	delete(m.status, name)
	m.mu.Unlock()
	if client == nil {
		return fmt.Errorf("MCP server not found: %s", name)
	}
	return client.Close()
}
func (m *Manager) Status() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Status, 0, len(m.status))
	for _, item := range m.status {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
func (m *Manager) setStatus(status Status) {
	m.mu.Lock()
	m.status[status.Name] = status
	m.mu.Unlock()
}

// Sanitize matches OpenCode's MCP catalog naming rule: only ASCII letters,
// digits, underscore, and hyphen survive. Every other rune becomes '_'.
func Sanitize(value string) string {
	result := []rune(value)
	for index, item := range result {
		if (item >= 'a' && item <= 'z') || (item >= 'A' && item <= 'Z') || (item >= '0' && item <= '9') || item == '_' || item == '-' {
			continue
		}
		result[index] = '_'
	}
	return string(result)
}

func ToolName(clientName, name string) string { return Sanitize(clientName) + "_" + Sanitize(name) }

func NormalizeInputSchema(input map[string]any) map[string]any {
	result := make(map[string]any, len(input)+3)
	for key, value := range input {
		result[key] = value
	}
	result["type"] = "object"
	if _, ok := result["properties"]; !ok {
		result["properties"] = map[string]any{}
	}
	result["additionalProperties"] = false
	return result
}
