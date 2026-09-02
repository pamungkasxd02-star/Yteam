package lsp

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Status struct {
	ID         string   `json:"id"`
	Root       string   `json:"root"`
	Status     string   `json:"status"`
	Extensions []string `json:"extensions,omitempty"`
	Error      string   `json:"error,omitempty"`
}
type clientEntry struct {
	client     *Client
	root       string
	extensions map[string]bool
}
type Manager struct {
	mu       sync.RWMutex
	clients  map[string]clientEntry
	statuses map[string]Status
}

func NewManager() *Manager {
	return &Manager{clients: map[string]clientEntry{}, statuses: map[string]Status{}}
}
func (m *Manager) Connect(ctx context.Context, id string, command []string, root, rootURI string) error {
	return m.ConnectForExtensions(ctx, id, command, root, rootURI, nil)
}
func (m *Manager) ConnectForExtensions(ctx context.Context, id string, command []string, root, rootURI string, extensions []string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	m.mu.RLock()
	existing, ok := m.clients[id]
	m.mu.RUnlock()
	if ok && filepath.Clean(existing.root) == filepath.Clean(absoluteRoot) {
		return nil
	}
	client, err := Start(ctx, command, absoluteRoot)
	if err != nil {
		m.set(Status{ID: id, Root: absoluteRoot, Status: "error", Extensions: normalizeExtensions(extensions), Error: err.Error()})
		return err
	}
	if err := client.Initialize(ctx, rootURI); err != nil {
		_ = client.Close()
		m.set(Status{ID: id, Root: absoluteRoot, Status: "error", Extensions: normalizeExtensions(extensions), Error: err.Error()})
		return err
	}
	m.mu.Lock()
	old := m.clients[id]
	normalized := normalizeExtensions(extensions)
	allowed := map[string]bool{}
	for _, extension := range normalized {
		allowed[extension] = true
	}
	m.clients[id] = clientEntry{client: client, root: absoluteRoot, extensions: allowed}
	m.statuses[id] = Status{ID: id, Root: absoluteRoot, Status: "connected", Extensions: normalized}
	m.mu.Unlock()
	if old.client != nil {
		_ = old.client.Close()
	}
	return nil
}
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	entry := m.clients[id]
	delete(m.clients, id)
	delete(m.statuses, id)
	m.mu.Unlock()
	if entry.client == nil {
		return nil
	}
	return entry.client.Close()
}
func (m *Manager) Status() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Status, 0, len(m.statuses))
	for _, item := range m.statuses {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
func (m *Manager) set(status Status) { m.mu.Lock(); m.statuses[status.ID] = status; m.mu.Unlock() }

func normalizeExtensions(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (m *Manager) selectClient(path, root string) *Client {
	extension := strings.ToLower(filepath.Ext(path))
	cleanRoot := ""
	if strings.TrimSpace(root) != "" {
		cleanRoot, _ = filepath.Abs(root)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var fallback *Client
	for _, entry := range m.clients {
		if entry.client == nil {
			continue
		}
		if cleanRoot != "" && filepath.Clean(entry.root) != filepath.Clean(cleanRoot) {
			continue
		}
		if fallback == nil {
			fallback = entry.client
		}
		if extension != "" && entry.extensions[extension] {
			return entry.client
		}
	}
	return fallback
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	entries := make([]clientEntry, 0, len(m.clients))
	for _, entry := range m.clients {
		entries = append(entries, entry)
	}
	m.clients = map[string]clientEntry{}
	m.statuses = map[string]Status{}
	m.mu.Unlock()
	var first error
	for _, entry := range entries {
		if entry.client == nil {
			continue
		}
		if err := entry.client.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
		if err := entry.client.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
