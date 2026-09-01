package session

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	path    string
}

func NewInputQueue() *InputQueue {
	return &InputQueue{items: map[string][]Input{}, seq: map[string]uint64{}, wake: map[string]chan struct{}{}, stopped: map[string]bool{}}
}

// OpenInputQueue replays the durable SessionInput journal used by the runtime.
// The journal is append-only so admitted and promoted state survives process
// restarts without requiring a database dependency.
func OpenInputQueue(home string) (*InputQueue, error) {
	queue := NewInputQueue()
	queue.path = filepath.Join(home, "session-inputs.jsonl")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	file, err := os.Open(queue.path)
	if errors.Is(err, os.ErrNotExist) {
		return queue, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record inputRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		queue.replay(record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return queue, nil
}

type inputRecord struct {
	Op    string `json:"op"`
	Input Input  `json:"input"`
}

func (q *InputQueue) replay(record inputRecord) {
	item := record.Input
	if item.Sequence > q.seq[item.SessionID] {
		q.seq[item.SessionID] = item.Sequence
	}
	switch record.Op {
	case "admit":
		for _, existing := range q.items[item.SessionID] {
			if existing.ID == item.ID {
				return
			}
		}
		q.items[item.SessionID] = append(q.items[item.SessionID], item)
	case "promote":
		for index := range q.items[item.SessionID] {
			if q.items[item.SessionID][index].ID == item.ID {
				q.items[item.SessionID][index].PromotedSeq = item.PromotedSeq
				return
			}
		}
	case "remove":
		items := q.items[item.SessionID]
		for index := range items {
			if items[index].ID == item.ID {
				q.items[item.SessionID] = append(items[:index], items[index+1:]...)
				return
			}
		}
	}
}

func (q *InputQueue) AppendOnly(path string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.path = path
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

func (q *InputQueue) persistLocked(record inputRecord) error {
	if q.path == "" {
		return nil
	}
	file, err := os.OpenFile(q.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(file).Encode(record)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
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
	if err := q.persistLocked(inputRecord{Op: "admit", Input: item}); err != nil {
		q.items[sessionID] = q.items[sessionID][:len(q.items[sessionID])-1]
		q.seq[sessionID]--
		return Input{}, err
	}
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
			_ = q.persistLocked(inputRecord{Op: "remove", Input: item})
			return true
		}
	}
	return false
}

func (q *InputQueue) Promote(sessionID string) ([]Input, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.items[sessionID]
	if len(items) == 0 {
		return nil, nil
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
		return nil, nil
	}
	for index := range items {
		for selectedIndex := range selected {
			if items[index].ID == selected[selectedIndex].ID {
				items[index].PromotedSeq = items[index].Sequence
				selected[selectedIndex].PromotedSeq = items[index].PromotedSeq
				if err := q.persistLocked(inputRecord{Op: "promote", Input: items[index]}); err != nil {
					return nil, err
				}
			}
		}
	}
	q.items[sessionID] = items
	return selected, nil
}

func (q *InputQueue) PromoteByID(id string) (Input, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for sessionID, items := range q.items {
		for index := range items {
			if items[index].ID == id && items[index].PromotedSeq == 0 {
				items[index].PromotedSeq = items[index].Sequence
				if err := q.persistLocked(inputRecord{Op: "promote", Input: items[index]}); err != nil {
					return Input{}, false, err
				}
				q.items[sessionID] = items
				return items[index], true, nil
			}
		}
	}
	return Input{}, false, nil
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
