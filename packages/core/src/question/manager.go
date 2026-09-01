package question

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

var ErrNotFound = errors.New("question request not found")
var ErrRejected = errors.New("question rejected")

type Manager struct {
	mu      sync.Mutex
	pending map[string]schema.QuestionRequest
	waiters map[string]chan result
}

type result struct {
	answers []schema.QuestionAnswer
	err     error
}

func NewManager() *Manager {
	return &Manager{pending: map[string]schema.QuestionRequest{}, waiters: map[string]chan result{}}
}

func (m *Manager) Ask(sessionID string, questions []schema.QuestionInfo, tool *schema.QuestionToolRef) (schema.QuestionRequest, error) {
	if sessionID == "" || len(questions) == 0 {
		return schema.QuestionRequest{}, errors.New("session and questions are required")
	}
	id := newID()
	request := schema.QuestionRequest{ID: id, SessionID: sessionID, Questions: questions, Tool: tool}
	m.mu.Lock()
	m.pending[id] = request
	m.waiters[id] = make(chan result, 1)
	m.mu.Unlock()
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
	return result
}

func (m *Manager) Reply(ctx context.Context, sessionID, id string, answers []schema.QuestionAnswer) error {
	m.mu.Lock()
	request, ok := m.pending[id]
	waiter := m.waiters[id]
	if !ok || request.SessionID != sessionID {
		m.mu.Unlock()
		return ErrNotFound
	}
	delete(m.pending, id)
	m.mu.Unlock()
	select {
	case waiter <- result{answers: answers}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) Reject(ctx context.Context, sessionID, id string) error {
	m.mu.Lock()
	request, ok := m.pending[id]
	waiter := m.waiters[id]
	if !ok || request.SessionID != sessionID {
		m.mu.Unlock()
		return ErrNotFound
	}
	delete(m.pending, id)
	m.mu.Unlock()
	select {
	case waiter <- result{err: ErrRejected}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) Await(ctx context.Context, id string) ([]schema.QuestionAnswer, error) {
	m.mu.Lock()
	waiter, ok := m.waiters[id]
	m.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-waiter:
		m.mu.Lock()
		delete(m.waiters, id)
		m.mu.Unlock()
		return response.answers, response.err
	}
}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "que_unknown"
	}
	return "que_" + hex.EncodeToString(buf)
}
