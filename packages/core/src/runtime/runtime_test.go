package runtime

import (
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
