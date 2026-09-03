package agent

import (
	"context"
	"sync"
	"time"
)
// Task represents a discrete piece of work executed by an agent.
type Task struct {
	ID          string `json:"id"`
	Agent       string `json:"agent"`
	Description string `json:"description"`
	Input       string `json:"input"`
}

// TaskResult holds the execution outcome of a subagent task.
type TaskResult struct {
	TaskID    string        `json:"task_id"`
	Agent     string        `json:"agent"`
	Output    string        `json:"output"`
	Error     error         `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
}

// Pipeline orchestrates concurrent subagents executing distinct tasks in parallel.
type Pipeline struct {
	maxWorkers int
}

func NewPipeline(maxWorkers int) *Pipeline {
	if maxWorkers <= 0 {
		maxWorkers = 4
	}
	return &Pipeline{maxWorkers: maxWorkers}
}

// ExecuteParallel runs multiple tasks concurrently across worker agents.
func (p *Pipeline) ExecuteParallel(ctx context.Context, tasks []Task, executeFn func(ctx context.Context, t Task) (string, error)) []TaskResult {
	results := make([]TaskResult, len(tasks))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, p.maxWorkers)

	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, task Task) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			start := time.Now()
			out, err := executeFn(ctx, task)
			dur := time.Since(start)

			results[idx] = TaskResult{
				TaskID:   task.ID,
				Agent:    task.Agent,
				Output:   out,
				Error:    err,
				Duration: dur,
			}
		}(i, t)
	}

	wg.Wait()
	return results
}
