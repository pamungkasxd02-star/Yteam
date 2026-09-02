package permission

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
var ErrNotFound = errors.New("permission request not found")

type Engine struct {
	mu      sync.Mutex
	rules   []Rule
	pending map[string]Request
	waiters map[string]chan error
	results map[string]error
	path    string
}

type record struct {
	Op      string   `json:"op"`
	Request *Request `json:"request,omitempty"`
	ID      string   `json:"id,omitempty"`
	Reply   Reply    `json:"reply,omitempty"`
}

func New(rules []Rule) *Engine { return newEngine(rules, "") }

// Open loads persistent permission requests and Always rules from home. The
// journal is append-only so a pending tool approval survives a process restart.
func Open(home string, rules []Rule) (*Engine, error) {
	path := filepath.Join(home, "permissions.jsonl")
	engine := newEngine(rules, path)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return engine, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var item record
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, err
		}
		if err := engine.replay(item); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return engine, nil
}

func newEngine(rules []Rule, path string) *Engine {
	return &Engine{rules: append([]Rule(nil), rules...), pending: map[string]Request{}, waiters: map[string]chan error{}, results: map[string]error{}, path: path}
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
	request := Request{ID: newID("per_"), SessionID: sessionID, Action: action, Resources: append([]string(nil), resources...)}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.persistLocked(record{Op: "ask", Request: &request, ID: request.ID}); err != nil {
		return Request{}, err
	}
	e.pending[request.ID] = request
	e.waiters[request.ID] = make(chan error, 1)
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
	request, ok := e.pending[id]
	if !ok {
		e.mu.Unlock()
		return ErrNotFound
	}
	if reply != Once && reply != Always && reply != Reject {
		e.mu.Unlock()
		return errors.New("unknown permission reply")
	}
	if err := e.persistLocked(record{Op: "reply", ID: id, Reply: reply}); err != nil {
		e.mu.Unlock()
		return err
	}
	delete(e.pending, id)
	if reply == Always {
		for _, resource := range request.Resources {
			e.rules = append(e.rules, Rule{Action: request.Action, Resource: resource, Effect: Allow})
		}
	}
	var response error
	if reply == Reject {
		response = ErrRejected
	}
	waiter := e.waiters[id]
	if waiter == nil {
		e.results[id] = response
	}
	e.mu.Unlock()
	if waiter == nil {
		return response
	}
	waiter <- response
	return response
}

func (e *Engine) Await(ctx context.Context, id string) error {
	e.mu.Lock()
	if response, ok := e.results[id]; ok {
		delete(e.results, id)
		e.mu.Unlock()
		return response
	}
	waiter, ok := e.waiters[id]
	if !ok {
		if _, pending := e.pending[id]; !pending {
			e.mu.Unlock()
			return ErrNotFound
		}
		waiter = make(chan error, 1)
		e.waiters[id] = waiter
	}
	e.mu.Unlock()
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
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (e *Engine) replay(item record) error {
	switch item.Op {
	case "ask":
		if item.Request == nil || item.Request.ID == "" {
			return errors.New("invalid permission ask record")
		}
		if _, done := e.results[item.Request.ID]; !done {
			e.pending[item.Request.ID] = *item.Request
		}
	case "reply":
		if item.ID == "" {
			return errors.New("invalid permission reply record")
		}
		request := e.pending[item.ID]
		delete(e.pending, item.ID)
		var response error
		if item.Reply == Reject {
			response = ErrRejected
		} else if item.Reply == Always {
			for _, resource := range request.Resources {
				e.rules = append(e.rules, Rule{Action: request.Action, Resource: resource, Effect: Allow})
			}
		}
		e.results[item.ID] = response
	default:
		return errors.New("unknown permission journal operation: " + item.Op)
	}
	return nil
}

func (e *Engine) persistLocked(item record) error {
	if e.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(e.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(file).Encode(item)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
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
