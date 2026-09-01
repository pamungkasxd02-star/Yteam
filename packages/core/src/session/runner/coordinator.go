package runner

import (
	"context"
	"sort"
	"sync"
)

// Coordinator enforces one active run per session while allowing unrelated
// sessions to run concurrently.
type Coordinator struct {
	mu     sync.Mutex
	active map[string]*execution
}

type execution struct {
	done chan struct{}
	err  error
}

func NewCoordinator() *Coordinator {
	return &Coordinator{active: map[string]*execution{}}
}

func (c *Coordinator) Run(ctx context.Context, sessionID string, work func(context.Context) error) error {
	c.mu.Lock()
	if current := c.active[sessionID]; current != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-current.done:
			return current.err
		}
	}
	current := &execution{done: make(chan struct{})}
	c.active[sessionID] = current
	c.mu.Unlock()

	current.err = work(ctx)
	c.mu.Lock()
	delete(c.active, sessionID)
	close(current.done)
	c.mu.Unlock()
	return current.err
}

func (c *Coordinator) Active() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, 0, len(c.active))
	for id := range c.active {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}
