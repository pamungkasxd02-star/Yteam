package permission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
)

type Effect string

const (
	Allow Effect = "allow"
	Deny  Effect = "deny"
	Ask   Effect = "ask"
)

type Rule struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Effect   Effect `json:"effect"`
}
type Reply string

const (
	Once   Reply = "once"
	Always Reply = "always"
	Reject Reply = "reject"
)

type Request struct {
	ID        string   `json:"id"`
	SessionID string   `json:"session_id"`
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
}

var ErrDenied = errors.New("permission denied")
var ErrRejected = errors.New("permission rejected")

type Engine struct {
	mu      sync.Mutex
	rules   []Rule
	pending map[string]Request
	waiters map[string]chan error
}

func New(rules []Rule) *Engine {
	return &Engine{rules: append([]Rule(nil), rules...), pending: map[string]Request{}, waiters: map[string]chan error{}}
}
func (e *Engine) Evaluate(action, resource string) Effect {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := Ask
	for _, rule := range e.rules {
		if match(rule.Action, action) && match(rule.Resource, resource) {
			result = rule.Effect
		}
	}
	return result
}
func (e *Engine) Assert(sessionID, action string, resources ...string) (Request, error) {
	result := Allow
	for _, resource := range resources {
		switch effect := e.Evaluate(action, resource); effect {
		case Deny:
			return Request{}, ErrDenied
		case Ask:
			result = Ask
		}
	}
	if result == Allow {
		return Request{}, nil
	}
	request := Request{ID: newID("per_"), SessionID: sessionID, Action: action, Resources: resources}
	e.mu.Lock()
	e.pending[request.ID] = request
	e.waiters[request.ID] = make(chan error, 1)
	e.mu.Unlock()
	return request, nil
}

func (e *Engine) Get(id string) (Request, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	request, ok := e.pending[id]
	return request, ok
}
func (e *Engine) Reply(id string, reply Reply) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	request, ok := e.pending[id]
	if !ok {
		return errors.New("permission request not found")
	}
	delete(e.pending, id)
	waiter := e.waiters[id]
	if reply == Reject {
		if waiter != nil {
			waiter <- ErrRejected
		}
		return ErrRejected
	}
	if reply == Always {
		for _, resource := range request.Resources {
			e.rules = append(e.rules, Rule{Action: request.Action, Resource: resource, Effect: Allow})
		}
	}
	if waiter != nil {
		waiter <- nil
	}
	return nil
}

// Await blocks a tool execution until a UI/server client replies to its
// pending permission request. Context cancellation removes the waiter but
// leaves no authorization behind.
func (e *Engine) Await(ctx context.Context, id string) error {
	e.mu.Lock()
	waiter, ok := e.waiters[id]
	e.mu.Unlock()
	if !ok {
		return errors.New("permission request not found")
	}
	select {
	case <-ctx.Done():
		e.mu.Lock()
		delete(e.waiters, id)
		e.mu.Unlock()
		return ctx.Err()
	case err := <-waiter:
		e.mu.Lock()
		delete(e.waiters, id)
		e.mu.Unlock()
		return err
	}
}
func (e *Engine) Pending() []Request {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]Request, 0, len(e.pending))
	for _, request := range e.pending {
		result = append(result, request)
	}
	return result
}
func match(pattern, value string) bool {
	if pattern == "*" || pattern == value {
		return true
	}
	if strings.ContainsAny(pattern, "*?") {
		ok, _ := filepath.Match(pattern, value)
		return ok
	}
	return false
}
func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "unknown"
	}
	return prefix + hex.EncodeToString(b[:])
}
