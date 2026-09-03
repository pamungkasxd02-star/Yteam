package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSubagentPipelineParallel(t *testing.T) {
	pipeline := NewPipeline(2)
	tasks := []Task{
		{ID: "t1", Agent: "explore", Description: "Explore API"},
		{ID: "t2", Agent: "explore", Description: "Explore DB"},
		{ID: "t3", Agent: "general", Description: "Draft Report"},
	}

	results := pipeline.ExecuteParallel(context.Background(), tasks, func(ctx context.Context, task Task) (string, error) {
		time.Sleep(10 * time.Millisecond)
		if task.ID == "t3" {
			return "", errors.New("draft failed")
		}
		return "result of " + task.Description, nil
	})

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Output != "result of Explore API" || results[1].Output != "result of Explore DB" {
		t.Fatalf("unexpected task output: %#v", results)
	}
	if results[2].Error == nil {
		t.Fatal("expected task 3 to fail")
	}
}
