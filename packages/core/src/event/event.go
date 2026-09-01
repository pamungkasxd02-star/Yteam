package event

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Journal struct {
	path string
	mu   sync.Mutex
	seq  map[string]uint64
	subs map[chan schema.Event]struct{}
}

func Open(home string) (*Journal, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	j := &Journal{path: filepath.Join(home, "events.jsonl"), seq: map[string]uint64{}, subs: map[chan schema.Event]struct{}{}}
	events, err := j.All()
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.Sequence > j.seq[event.Aggregate] {
			j.seq[event.Aggregate] = event.Sequence
		}
	}
	return j, nil
}

func (j *Journal) Publish(_ context.Context, typ, aggregate string, data map[string]any) (schema.Event, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	seq := j.seq[aggregate] + 1
	j.seq[aggregate] = seq
	e := schema.Event{ID: id("evt_"), Type: typ, Aggregate: aggregate, Sequence: seq, Version: 1, Data: data, CreatedAt: time.Now().UTC()}
	file, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return schema.Event{}, err
	}
	err = json.NewEncoder(file).Encode(e)
	_ = file.Close()
	if err != nil {
		return schema.Event{}, err
	}
	for sub := range j.subs {
		select {
		case sub <- e:
		default:
		}
	}
	return e, nil
}

func (j *Journal) All() ([]schema.Event, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	file, err := os.Open(j.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []schema.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var e schema.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, scanner.Err()
}

func (j *Journal) Since(aggregate string, after uint64) ([]schema.Event, error) {
	events, err := j.All()
	if err != nil {
		return nil, err
	}
	result := make([]schema.Event, 0)
	for _, item := range events {
		if item.Aggregate == aggregate && item.Sequence > after {
			result = append(result, item)
		}
	}
	return result, nil
}

func (j *Journal) Subscribe(ctx context.Context) <-chan schema.Event {
	j.mu.Lock()
	ch := make(chan schema.Event, 128)
	j.subs[ch] = struct{}{}
	j.mu.Unlock()
	go func() { <-ctx.Done(); j.mu.Lock(); delete(j.subs, ch); close(ch); j.mu.Unlock() }()
	return ch
}

func id(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return prefix + "unknown"
	}
	return prefix + hex.EncodeToString(b)
}
