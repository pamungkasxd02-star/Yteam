package tui

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const MaxHistoryEntries = 50

type PromptEntry struct {
	Input string `json:"input"`
	Mode  string `json:"mode,omitempty"`
	Parts []any  `json:"parts,omitempty"`
}

type PromptHistory struct {
	mu      sync.Mutex
	path    string
	entries []PromptEntry
	cursor  int
	draft   string
}

func OpenPromptHistory(home string) (*PromptHistory, error) {
	if home == "" {
		return NewPromptHistory(""), nil
	}
	history := NewPromptHistory(filepath.Join(home, "prompt-history.jsonl"))
	file, err := os.Open(history.path)
	if errors.Is(err, os.ErrNotExist) {
		return history, nil
	}
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry PromptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || entry.Input == "" {
			continue
		}
		history.entries = append(history.entries, entry)
	}
	if err := scanner.Err(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if len(history.entries) > MaxHistoryEntries {
		history.entries = history.entries[len(history.entries)-MaxHistoryEntries:]
	}
	history.cursor = len(history.entries)
	if err := history.rewrite(); err != nil {
		return nil, err
	}
	return history, nil
}

func (h *PromptHistory) Path() string { return h.path }

func NewPromptHistory(path string) *PromptHistory {
	return &PromptHistory{path: path, cursor: 0}
}

func (h *PromptHistory) Entries() []PromptEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]PromptEntry(nil), h.entries...)
}

func (h *PromptHistory) Append(input string) error {
	if input == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	entry := PromptEntry{Input: input}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1].Input == entry.Input && h.entries[len(h.entries)-1].Mode == entry.Mode {
		h.cursor = len(h.entries)
		h.draft = ""
		return nil
	}
	h.entries = append(h.entries, entry)
	trimmed := false
	if len(h.entries) > MaxHistoryEntries {
		h.entries = h.entries[len(h.entries)-MaxHistoryEntries:]
		trimmed = true
	}
	h.cursor = len(h.entries)
	h.draft = ""
	if trimmed {
		return h.rewriteLocked()
	}
	return h.appendLocked(entry)
}

func (h *PromptHistory) Move(direction int, input string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 || direction == 0 {
		return "", false
	}
	if h.cursor == len(h.entries) {
		if input != "" {
			h.draft = input
		}
	} else if input != h.entries[h.cursor].Input && input != "" {
		return "", false
	}
	next := h.cursor + direction
	if next < 0 {
		next = 0
	}
	if next >= len(h.entries) {
		h.cursor = len(h.entries)
		return h.draft, true
	}
	h.cursor = next
	return h.entries[next].Input, true
}

func (h *PromptHistory) ResetNavigation() {
	h.mu.Lock()
	h.cursor = len(h.entries)
	h.draft = ""
	h.mu.Unlock()
}

func (h *PromptHistory) appendLocked(entry PromptEntry) error {
	if h.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(h.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(file).Encode(entry)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (h *PromptHistory) rewrite() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rewriteLocked()
}

func (h *PromptHistory) rewriteLocked() error {
	if h.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return err
	}
	tmp := h.path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	for _, entry := range h.entries {
		if err := json.NewEncoder(file).Encode(entry); err != nil {
			_ = file.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Windows cannot replace an existing file with Rename when the target is
	// still open elsewhere; the journal is already serialized by h.mu.
	_ = os.Remove(h.path)
	return os.Rename(tmp, h.path)
}
