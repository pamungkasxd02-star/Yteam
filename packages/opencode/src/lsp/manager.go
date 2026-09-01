package lsp

import (
	"context"
	"sync"
)

type Status struct {
	ID     string `json:"id"`
	Root   string `json:"root"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
type Manager struct {
	mu       sync.RWMutex
	clients  map[string]*Client
	statuses map[string]Status
}

func NewManager() *Manager {
	return &Manager{clients: map[string]*Client{}, statuses: map[string]Status{}}
}
func (m *Manager) Connect(ctx context.Context, id string, command []string, root, rootURI string) error {
	client, err := Start(ctx, command, root)
	if err != nil {
		m.set(Status{ID: id, Root: root, Status: "error", Error: err.Error()})
		return err
	}
	if err := client.Initialize(ctx, rootURI); err != nil {
		_ = client.Close()
		m.set(Status{ID: id, Root: root, Status: "error", Error: err.Error()})
		return err
	}
	m.mu.Lock()
	m.clients[id] = client
	m.statuses[id] = Status{ID: id, Root: root, Status: "connected"}
	m.mu.Unlock()
	return nil
}
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	client := m.clients[id]
	delete(m.clients, id)
	delete(m.statuses, id)
	m.mu.Unlock()
	if client == nil {
		return nil
	}
	return client.Close()
}
func (m *Manager) Status() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Status, 0, len(m.statuses))
	for _, item := range m.statuses {
		result = append(result, item)
	}
	return result
}
func (m *Manager) set(status Status) { m.mu.Lock(); m.statuses[status.ID] = status; m.mu.Unlock() }
