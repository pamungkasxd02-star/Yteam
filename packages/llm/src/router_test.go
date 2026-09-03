package llm

import (
	"context"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

type mockAdapter struct {
	name  string
	pType ProviderType
}

func (m *mockAdapter) Name() string        { return m.name }
func (m *mockAdapter) Type() ProviderType { return m.pType }
func (m *mockAdapter) Models(ctx context.Context) ([]protocol.Model, error) {
	return []protocol.Model{{ID: "mock-model", Name: "Mock"}}, nil
}
func (m *mockAdapter) Complete(ctx context.Context, req protocol.ChatRequest, emit func(StreamDelta) error) error {
	return emit(StreamDelta{Content: "mock response"})
}

func TestRouterSelection(t *testing.T) {
	fb := &mockAdapter{name: "OpenAI", pType: ProviderOpenAI}
	anth := &mockAdapter{name: "Anthropic", pType: ProviderAnthropic}
	gem := &mockAdapter{name: "Gemini", pType: ProviderGemini}
	oll := &mockAdapter{name: "Ollama", pType: ProviderOllama}

	router := NewRouter(fb)
	router.Register(ProviderAnthropic, anth)
	router.Register(ProviderGemini, gem)
	router.Register(ProviderOllama, oll)

	if a := router.Select("claude-3-7-sonnet"); a.Type() != ProviderAnthropic {
		t.Fatalf("expected Anthropic, got %v", a.Type())
	}
	if a := router.Select("gemini-2.5-pro"); a.Type() != ProviderGemini {
		t.Fatalf("expected Gemini, got %v", a.Type())
	}
	if a := router.Select("ollama/llama3"); a.Type() != ProviderOllama {
		t.Fatalf("expected Ollama, got %v", a.Type())
	}
	if a := router.Select("gpt-4o"); a.Type() != ProviderOpenAI {
		t.Fatalf("expected OpenAI fallback, got %v", a.Type())
	}
}

func TestRouterExecution(t *testing.T) {
	fb := &mockAdapter{name: "OpenAI", pType: ProviderOpenAI}
	router := NewRouter(fb)

	var output string
	err := router.Complete(context.Background(), protocol.ChatRequest{Model: "gpt-4o"}, func(d StreamDelta) error {
		output += d.Content
		return nil
	})
	if err != nil || output != "mock response" {
		t.Fatalf("execution failed: err=%v, out=%q", err, output)
	}
}
