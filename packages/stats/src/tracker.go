package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionStats records aggregated metrics for a single or global execution.
type SessionStats struct {
	TotalTokens    int       `json:"total_tokens"`
	PromptTokens   int       `json:"prompt_tokens"`
	ResponseTokens int       `json:"response_tokens"`
	ToolInvocations int      `json:"tool_invocations"`
	RunCount       int       `json:"run_count"`
	LastActive     time.Time `json:"last_active"`
}

// Tracker manages persistence and aggregation of usage metrics.
type Tracker struct {
	mu       sync.Mutex
	filePath string
	stats    SessionStats
}

func Open(homeDir string) (*Tracker, error) {
	if homeDir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		homeDir = filepath.Join(h, ".config", "yteam")
	}
	_ = os.MkdirAll(homeDir, 0o700)
	p := filepath.Join(homeDir, "stats.json")

	t := &Tracker{filePath: p}
	data, err := os.ReadFile(p)
	if err == nil {
		_ = json.Unmarshal(data, &t.stats)
	}
	return t, nil
}

func (t *Tracker) RecordRun(promptTokens, responseTokens, toolCalls int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stats.RunCount++
	t.stats.PromptTokens += promptTokens
	t.stats.ResponseTokens += responseTokens
	t.stats.TotalTokens += (promptTokens + responseTokens)
	t.stats.ToolInvocations += toolCalls
	t.stats.LastActive = time.Now().UTC()

	data, err := json.MarshalIndent(t.stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(t.filePath, data, 0o600)
}

func (t *Tracker) Snapshot() SessionStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stats
}
