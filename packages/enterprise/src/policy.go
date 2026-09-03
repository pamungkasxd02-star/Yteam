package enterprise

import (
	"strings"
	"sync"
)

// Role represents an enterprise RBAC role (e.g. admin, developer, auditor, viewer).
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleAuditor   Role = "auditor"
	RoleViewer    Role = "viewer"
)

// Policy enforces corporate access boundaries, allowed models, and tool restrictions.
type Policy struct {
	AllowedModels    []string          `json:"allowed_models"`
	BlockedTools     []string          `json:"blocked_tools"`
	RequireApproval  bool              `json:"require_approval"`
	AllowedRoots     []string          `json:"allowed_roots"`
	RolePermissions  map[Role][]string `json:"role_permissions"`
}

// Manager validates actions against enterprise policy.
type Manager struct {
	mu     sync.RWMutex
	policy Policy
}

func NewManager(policy Policy) *Manager {
	if policy.RolePermissions == nil {
		policy.RolePermissions = map[Role][]string{
			RoleAdmin:     {"*"},
			RoleDeveloper: {"read", "write", "edit", "glob", "grep", "bash", "models", "session"},
			RoleAuditor:   {"read", "glob", "grep", "session:export", "audit:read"},
			RoleViewer:    {"read", "glob", "grep"},
		}
	}
	return &Manager{policy: policy}
}

func (m *Manager) IsModelAllowed(modelID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.policy.AllowedModels) == 0 {
		return true
	}
	for _, mName := range m.policy.AllowedModels {
		if mName == "*" || strings.EqualFold(mName, modelID) {
			return true
		}
	}
	return false
}

func (m *Manager) IsToolAllowed(role Role, toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, blocked := range m.policy.BlockedTools {
		if strings.EqualFold(blocked, toolName) {
			return false
		}
	}

	perms, ok := m.policy.RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == "*" || strings.EqualFold(p, toolName) {
			return true
		}
	}
	return false
}
