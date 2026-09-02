package plugin

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
	clients map[string]*Client
	status  map[string]Status
}

func NewManager() *Manager {
	return &Manager{clients: map[string]*Client{}, status: map[string]Status{}}
}
func (m *Manager) Connect(ctx context.Context, app *runtime.Runtime, name string, cfg Config) error {
	client, err := Start(ctx, cfg)
	if err != nil {
		m.set(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	if err := client.Initialize(ctx, name); err != nil {
		_ = client.Close()
		m.set(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	tools, err := client.Tools(ctx)
	if err != nil {
		_ = client.Close()
		m.set(Status{Name: name, Status: "failed", Error: err.Error()})
		return err
	}
	for _, item := range tools {
		if err := app.AddExternalToolNamed(client, name+"_"+item.Name, item.Name, item.Description, item.InputSchema); err != nil {
			_ = client.Close()
			m.set(Status{Name: name, Status: "failed", Error: err.Error()})
			return err
		}
	}
	m.mu.Lock()
	old := m.clients[name]
	m.clients[name] = client
	m.status[name] = Status{Name: name, Status: "connected", Tools: len(tools)}
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}
func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	client := m.clients[name]
	delete(m.clients, name)
	delete(m.status, name)
	m.mu.Unlock()
	if client == nil {
		return fmt.Errorf("plugin not found: %s", name)
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
func (m *Manager) set(status Status) { m.mu.Lock(); m.status[status.Name] = status; m.mu.Unlock() }
