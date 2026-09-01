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
	done        chan struct{}
	err         error
	pendingWake bool
	stopping    bool
	cancel      context.CancelFunc
	work        func(context.Context) error
}

func NewCoordinator() *Coordinator {
	return &Coordinator{active: map[string]*execution{}}
}

func (c *Coordinator) Run(ctx context.Context, sessionID string, work func(context.Context) error) error {
	for {
		c.mu.Lock()
		current := c.active[sessionID]
		if current == nil {
			current = &execution{done: make(chan struct{}), work: work}
			c.active[sessionID] = current
			c.startLocked(sessionID, current, ctx)
		}
		if current.stopping {
			done := current.done
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				continue
			}
		}
		done := current.done
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return current.err
		}
	}
}

// Wake records one follow-up execution for a session. Repeated wakes while a
// run is active are coalesced into one successor, matching OpenCode's
// run-coordinator contract.
func (c *Coordinator) Wake(sessionID string, work func(context.Context) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if current := c.active[sessionID]; current != nil {
		current.pendingWake = true
		return
	}
	current := &execution{done: make(chan struct{}), work: work}
	c.active[sessionID] = current
	c.startLocked(sessionID, current, context.Background())
}

func (c *Coordinator) Interrupt(ctx context.Context, sessionID string) error {
	c.mu.Lock()
	current := c.active[sessionID]
	if current == nil {
		c.mu.Unlock()
		return nil
	}
	current.stopping = true
	current.pendingWake = false
	cancel := current.cancel
	done := current.done
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (c *Coordinator) startLocked(sessionID string, current *execution, parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	current.cancel = cancel
	go func() {
		err := current.work(ctx)
		c.finish(sessionID, current, err)
	}()
}

func (c *Coordinator) finish(sessionID string, current *execution, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active[sessionID] != current {
		return
	}
	if current.pendingWake && err == nil && !current.stopping {
		current.pendingWake = false
		c.startLocked(sessionID, current, context.Background())
		return
	}
	if current.pendingWake {
		next := &execution{done: make(chan struct{}), work: current.work}
		c.active[sessionID] = next
		current.err = err
		close(current.done)
		c.startLocked(sessionID, next, context.Background())
		return
	}
	current.err = err
	delete(c.active, sessionID)
	close(current.done)
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
