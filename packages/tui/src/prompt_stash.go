package tui

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

const MaxStashEntries = 50

type StashEntry struct {
	Input     string               `json:"input"`
	Parts     []schema.MessagePart `json:"parts,omitempty"`
	Timestamp int64                `json:"timestamp"`
}

type PromptStash struct {
	mu      sync.Mutex
	path    string
	entries []StashEntry
}

func OpenPromptStash(home string) (*PromptStash, error) {
	stash := &PromptStash{path: filepath.Join(home, "prompt-stash.jsonl")}
	if home == "" {
		stash.path = ""
	}
	if stash.path == "" {
		return stash, nil
	}
	file, err := os.Open(stash.path)
	if errors.Is(err, os.ErrNotExist) {
		return stash, nil
	}
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry StashEntry
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.Input != "" {
			stash.entries = append(stash.entries, entry)
		}
	}
	closeErr := file.Close()
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(stash.entries) > MaxStashEntries {
		stash.entries = stash.entries[len(stash.entries)-MaxStashEntries:]
	}
	if err := stash.rewrite(); err != nil {
		return nil, err
	}
	return stash, nil
}

func (s *PromptStash) Entries() []StashEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]StashEntry, len(s.entries))
	copy(result, s.entries)
	for index := range result {
		result[index].Parts = clonePromptParts(result[index].Parts)
	}
	return result
}

func (s *PromptStash) Push(entry StashEntry) error {
	if entry.Input == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.Timestamp = time.Now().UnixMilli()
	entry.Parts = clonePromptParts(entry.Parts)
	s.entries = append(s.entries, entry)
	if len(s.entries) > MaxStashEntries {
		s.entries = s.entries[len(s.entries)-MaxStashEntries:]
		return s.rewriteLocked()
	}
	return s.appendLocked(entry)
}

func (s *PromptStash) Pop() (StashEntry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 {
		return StashEntry{}, false, nil
	}
	index := len(s.entries) - 1
	entry := s.entries[index]
	s.entries = s.entries[:index]
	return entry, true, s.rewriteLocked()
}

func (s *PromptStash) Remove(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.entries) {
		return nil
	}
	s.entries = append(s.entries[:index], s.entries[index+1:]...)
	return s.rewriteLocked()
}

func (s *PromptStash) appendLocked(entry StashEntry) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(entry)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func (s *PromptStash) rewrite() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rewriteLocked()
}

func (s *PromptStash) rewriteLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	for _, entry := range s.entries {
		if err := json.NewEncoder(file).Encode(entry); err != nil {
			_ = file.Close()
			_ = os.Remove(temporary)
			return err
		}
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	_ = os.Remove(s.path)
	return os.Rename(temporary, s.path)
}
