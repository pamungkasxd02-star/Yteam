package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultKeepMessages = 12
const MaxToolOutput = 2000

type Compaction struct {
	Summary   string    `json:"summary"`
	Recent    []Message `json:"recent"`
	CreatedAt string    `json:"created_at"`
}

func (s *Store) CompactMessages(id, summary string, keep int) (*Compaction, error) {
	if strings.TrimSpace(summary) == "" {
		return nil, errors.New("summary is empty")
	}
	if keep <= 0 {
		keep = DefaultKeepMessages
	}
	sess, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	start := len(sess.Messages) - keep
	if start < 0 {
		start = 0
	}
	recent := append([]Message(nil), sess.Messages[start:]...)
	for index := range recent {
		if recent[index].Role == "tool" && len(recent[index].Content) > MaxToolOutput {
			recent[index].Content = recent[index].Content[:MaxToolOutput] + "\n[truncated]"
		}
	}
	compaction := &Compaction{Summary: strings.TrimSpace(summary), Recent: recent, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := s.replaceMessages(id, []Message{{Role: "system", Content: "[context summary]\n" + compaction.Summary, CreatedAt: compaction.CreatedAt}}, recent); err != nil {
		return nil, err
	}
	return compaction, nil
}

func (s *Store) replaceMessages(id string, messages ...[]Message) error {
	if err := validID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.sessions, id+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, group := range messages {
		for _, message := range group {
			if err := json.NewEncoder(file).Encode(message); err != nil {
				return err
			}
		}
	}
	return nil
}
