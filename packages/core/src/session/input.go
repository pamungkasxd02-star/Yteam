package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Delivery string

const (
	DeliveryQueue Delivery = "queue"
	DeliverySteer Delivery = "steer"
)

type Input struct {
	ID          string   `json:"id"`
	SessionID   string   `json:"session_id"`
	Content     string   `json:"content"`
	Delivery    Delivery `json:"delivery"`
	CreatedAt   string   `json:"created_at"`
	Sequence    uint64   `json:"sequence"`
	PromotedSeq uint64   `json:"promoted_seq,omitempty"`
}

type InputQueue struct {
	mu      sync.Mutex
	items   map[string][]Input
	seq     map[string]uint64
	wake    map[string]chan struct{}
	stopped map[string]bool
}

func NewInputQueue() *InputQueue {
	return &InputQueue{items: map[string][]Input{}, seq: map[string]uint64{}, wake: map[string]chan struct{}{}, stopped: map[string]bool{}}
}

func (q *InputQueue) Admit(sessionID, content string, delivery Delivery) (Input, error) {
	if sessionID == "" {
		return Input{}, errors.New("session id is empty")
	}
	if content == "" {
		return Input{}, errors.New("input content is empty")
	}
	if delivery != DeliverySteer {
		delivery = DeliveryQueue
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq[sessionID]++
	item := Input{ID: newID("msg_"), SessionID: sessionID, Content: content, Delivery: delivery, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Sequence: q.seq[sessionID]}
	q.items[sessionID] = append(q.items[sessionID], item)
	if q.wake[sessionID] != nil {
		select {
		case q.wake[sessionID] <- struct{}{}:
		default:
		}
	}
	return item, nil
}

func (q *InputQueue) Pending(sessionID string) []Input {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]Input, 0, len(q.items[sessionID]))
	for _, item := range q.items[sessionID] {
		if item.PromotedSeq == 0 {
			result = append(result, item)
		}
	}
	return result
}

func (q *InputQueue) Remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for sessionID, items := range q.items {
		for index, item := range items {
			if item.ID != id {
				continue
			}
			q.items[sessionID] = append(items[:index], items[index+1:]...)
			return true
		}
	}
	return false
}

func (q *InputQueue) Promote(sessionID string) []Input {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.items[sessionID]
	if len(items) == 0 {
		return nil
	}
	steer := make([]Input, 0)
	queueIndex := -1
	for index, item := range items {
		if item.PromotedSeq != 0 {
			continue
		}
		if item.Delivery == DeliverySteer {
			steer = append(steer, item)
		} else if queueIndex == -1 {
			queueIndex = index
		}
	}
	selected := steer
	if queueIndex >= 0 {
		selected = append(selected, items[queueIndex])
	}
	if len(selected) == 0 {
		return nil
	}
	for index := range items {
		for selectedIndex := range selected {
			if items[index].ID == selected[selectedIndex].ID {
				items[index].PromotedSeq = items[index].Sequence
				selected[selectedIndex].PromotedSeq = items[index].PromotedSeq
			}
		}
	}
	q.items[sessionID] = items
	return selected
}

func (q *InputQueue) PromoteByID(id string) (Input, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for sessionID, items := range q.items {
		for index := range items {
			if items[index].ID == id && items[index].PromotedSeq == 0 {
				items[index].PromotedSeq = items[index].Sequence
				q.items[sessionID] = items
				return items[index], true
			}
		}
	}
	return Input{}, false
}

func (q *InputQueue) Wait(ctx context.Context, sessionID string) error {
	q.mu.Lock()
	channel := q.wake[sessionID]
	if channel == nil {
		channel = make(chan struct{}, 1)
		q.wake[sessionID] = channel
	}
	q.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-channel:
		return nil
	}
}

func (q *InputQueue) Interrupt(sessionID string) {
	q.mu.Lock()
	q.stopped[sessionID] = true
	if q.wake[sessionID] != nil {
		select {
		case q.wake[sessionID] <- struct{}{}:
		default:
		}
	}
	q.mu.Unlock()
}

func (q *InputQueue) ClearInterrupt(sessionID string) {
	q.mu.Lock()
	delete(q.stopped, sessionID)
	q.mu.Unlock()
}
func (q *InputQueue) IsInterrupted(sessionID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.stopped[sessionID]
}

func newID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return prefix + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + hex.EncodeToString(buf)
}
