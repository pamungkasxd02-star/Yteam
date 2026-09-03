package enterprise

import (
	"testing"
)

func TestEnterprisePolicy(t *testing.T) {
	mgr := NewManager(Policy{
		AllowedModels: []string{"claude-3-7-sonnet", "gpt-4o"},
		BlockedTools:  []string{"bash"},
	})

	if !mgr.IsModelAllowed("claude-3-7-sonnet") {
		t.Fatal("expected claude-3-7-sonnet to be allowed")
	}
	if mgr.IsModelAllowed("random-unapproved-model") {
		t.Fatal("expected unapproved model to be blocked")
	}

	if mgr.IsToolAllowed(RoleDeveloper, "bash") {
		t.Fatal("expected bash to be blocked by BlockedTools policy")
	}
	if !mgr.IsToolAllowed(RoleDeveloper, "read") {
		t.Fatal("expected read to be allowed for developer")
	}
	if mgr.IsToolAllowed(RoleViewer, "write") {
		t.Fatal("expected write to be blocked for viewer")
	}
}
