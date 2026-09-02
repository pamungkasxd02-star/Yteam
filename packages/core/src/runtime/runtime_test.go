package runtime

import (
	"strings"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func testRuntime(t *testing.T) *Runtime {
	t.Helper()
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	return New(config.Config{Home: home, Model: "initial"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
}

func TestRuntimeAgentAndModelSelection(t *testing.T) {
	r := testRuntime(t)
	if r.AgentName() != "build" || r.ModelName() != "initial" {
		t.Fatalf("initial state: %s %s", r.AgentName(), r.ModelName())
	}
	if err := r.SetAgent("plan"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetModel("new-model"); err != nil {
		t.Fatal(err)
	}
	if r.AgentName() != "plan" || r.ModelName() != "new-model" {
		t.Fatalf("selected state: %s %s", r.AgentName(), r.ModelName())
	}
	if err := r.SetAgent("missing"); err == nil {
		t.Fatal("expected unknown-agent error")
	}
	if err := r.SetModel(" "); err == nil {
		t.Fatal("expected empty-model error")
	}
}

func TestRuntimeAgentChangesPromptPersistsAndRestrictsTools(t *testing.T) {
	r := testRuntime(t)
	if err := r.SetAgent("plan"); err != nil {
		t.Fatal(err)
	}
	if r.Runner.Agent != "plan" || !strings.Contains(r.SystemPrompt(), "plan mode") {
		t.Fatalf("agent state = %q, prompt = %q", r.Runner.Agent, r.SystemPrompt())
	}
	loaded, err := r.Store.Load(r.CurrentSession().ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Agent != "plan" {
		t.Fatalf("persisted agent = %q", loaded.Agent)
	}
	definitions := r.Runner.ToolDefinitions()
	for _, definition := range definitions {
		if definition.Function.Name == "bash" || definition.Function.Name == "write" {
			t.Fatalf("mutating tool exposed in plan mode: %q", definition.Function.Name)
		}
	}
	if len(definitions) == 0 {
		t.Fatal("plan mode has no read tools")
	}
}

func TestRuntimeRestoresPersistedAgentOnConstruction(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	current, err = store.SetAgent(current.ID, "plan")
	if err != nil {
		t.Fatal(err)
	}
	r := New(config.Config{Home: home, Model: "test", Agent: "build"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	if r.AgentName() != "plan" || r.Runner.Agent != "plan" {
		t.Fatalf("restored agent = %q/%q", r.AgentName(), r.Runner.Agent)
	}
}
