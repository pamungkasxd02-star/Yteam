package question

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
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

var ErrNotFound = errors.New("question request not found")
var ErrRejected = errors.New("question rejected")

type Manager struct {
	mu      sync.Mutex
	pending map[string]schema.QuestionRequest
	waiters map[string]chan result
	results map[string]result
	path    string
}

type result struct {
	Answers []schema.QuestionAnswer `json:"answers,omitempty"`
	Error   string                  `json:"error,omitempty"`
}

type record struct {
	Op      string                  `json:"op"`
	Request *schema.QuestionRequest `json:"request,omitempty"`
	ID      string                  `json:"id"`
	Answers []schema.QuestionAnswer `json:"answers,omitempty"`
}

func NewManager() *Manager {
	return newManager("")
}

// OpenManager replays the durable question journal below application home.
// Existing callers can keep NewManager for in-memory operation and tests.
func OpenManager(home string) (*Manager, error) {
	manager := newManager(filepath.Join(home, "questions.jsonl"))
	if home == "" {
		return manager, nil
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	file, err := os.Open(manager.path)
	if errors.Is(err, os.ErrNotExist) {
		return manager, nil
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
		if err := manager.replay(item); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return manager, nil
}

func newManager(path string) *Manager {
	return &Manager{pending: map[string]schema.QuestionRequest{}, waiters: map[string]chan result{}, results: map[string]result{}, path: path}
}

func (m *Manager) Ask(sessionID string, questions []schema.QuestionInfo, tool *schema.QuestionToolRef) (schema.QuestionRequest, error) {
	if sessionID == "" || len(questions) == 0 {
		return schema.QuestionRequest{}, errors.New("session and questions are required")
	}
	request := schema.QuestionRequest{ID: newID(), SessionID: sessionID, Questions: questions, Tool: tool}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.persistLocked(record{Op: "ask", Request: &request, ID: request.ID}); err != nil {
		return schema.QuestionRequest{}, err
	}
	m.pending[request.ID] = request
	m.waiters[request.ID] = make(chan result, 1)
	return request, nil
}

func (m *Manager) Pending(sessionID string) []schema.QuestionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]schema.QuestionRequest, 0)
	for _, request := range m.pending {
		if sessionID == "" || request.SessionID == sessionID {
			result = append(result, request)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Manager) Reply(ctx context.Context, sessionID, id string, answers []schema.QuestionAnswer) error {
	return m.finish(ctx, sessionID, id, result{Answers: answers}, record{Op: "reply", ID: id, Answers: answers})
}

func (m *Manager) Reject(ctx context.Context, sessionID, id string) error {
	return m.finish(ctx, sessionID, id, result{Error: ErrRejected.Error()}, record{Op: "reject", ID: id})
}

func (m *Manager) finish(ctx context.Context, sessionID, id string, response result, entry record) error {
	m.mu.Lock()
	request, ok := m.pending[id]
	if !ok || request.SessionID != sessionID {
		m.mu.Unlock()
		return ErrNotFound
	}
	if err := m.persistLocked(entry); err != nil {
		m.mu.Unlock()
		return err
	}
	delete(m.pending, id)
	waiter := m.waiters[id]
	if waiter == nil {
		m.results[id] = response
	}
	m.mu.Unlock()
	if waiter == nil {
		return nil
	}
	select {
	case waiter <- response:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) Await(ctx context.Context, id string) ([]schema.QuestionAnswer, error) {
	m.mu.Lock()
	if response, ok := m.results[id]; ok {
		delete(m.results, id)
		m.mu.Unlock()
		return response.Answers, responseError(response)
	}
	waiter, ok := m.waiters[id]
	if !ok {
		if _, pending := m.pending[id]; !pending {
			m.mu.Unlock()
			return nil, ErrNotFound
		}
		waiter = make(chan result, 1)
		m.waiters[id] = waiter
	}
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-waiter:
		m.mu.Lock()
		delete(m.waiters, id)
		m.mu.Unlock()
		return response.Answers, responseError(response)
	}
}

func responseError(response result) error {
	if response.Error == ErrRejected.Error() {
		return ErrRejected
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	return nil
}

func (m *Manager) replay(item record) error {
	switch item.Op {
	case "ask":
		if item.Request == nil || item.Request.ID == "" || item.Request.SessionID == "" {
			return errors.New("invalid question ask record")
		}
		if _, exists := m.pending[item.Request.ID]; !exists {
			if _, done := m.results[item.Request.ID]; !done {
				m.pending[item.Request.ID] = *item.Request
			}
		}
	case "reply":
		if item.ID == "" {
			return errors.New("invalid question reply record")
		}
		delete(m.pending, item.ID)
		m.results[item.ID] = result{Answers: item.Answers}
	case "reject":
		if item.ID == "" {
			return errors.New("invalid question reject record")
		}
		delete(m.pending, item.ID)
		m.results[item.ID] = result{Error: ErrRejected.Error()}
	default:
		return errors.New("unknown question journal operation: " + item.Op)
	}
	return nil
}

func (m *Manager) persistLocked(item record) error {
	if m.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(m.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "que_unknown"
	}
	return "que_" + hex.EncodeToString(buf)
}
