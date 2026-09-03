package session

import (
	"strings"
	"testing"
)

func TestContextBudgetEstimationAndPlan(t *testing.T) {
	budget := NewContextBudget(1000)
	if budget.CompactionThreshold != 800 {
		t.Fatalf("unexpected threshold: %d", budget.CompactionThreshold)
	}

	messages := []Message{
		{ID: "m1", Role: "user", Content: strings.Repeat("a", 1000)},
		{ID: "m2", Role: "assistant", Content: strings.Repeat("b", 1000)},
		{ID: "m3", Role: "user", Content: strings.Repeat("c", 1000)},
		{ID: "m4", Role: "assistant", Content: strings.Repeat("d", 1000)},
		{ID: "m5", Role: "user", Content: "recent 1"},
		{ID: "m6", Role: "assistant", Content: "recent 2"},
		{ID: "m7", Role: "user", Content: "recent 3"},
		{ID: "m8", Role: "assistant", Content: "recent 4"},
	}

	tokens := budget.EstimateTokens(messages)
	if tokens < 1000 {
		t.Fatalf("unexpected token estimate: %d", tokens)
	}

	if !budget.ShouldCompact(messages) {
		t.Fatal("expected ShouldCompact to be true")
	}

	summary, recent := budget.PlanCompaction(messages)
	if !strings.Contains(summary, "Previous context summary") {
		t.Fatalf("unexpected summary: %s", summary)
	}
	if len(recent) != 6 {
		t.Fatalf("expected 6 recent messages kept, got %d", len(recent))
	}
}
