package session

import (
	"strings"
)

// ContextBudget calculates token consumption and decides when auto-compaction is needed.
type ContextBudget struct {
	MaxTokens        int `json:"max_tokens"`
	CompactionThreshold int `json:"compaction_threshold"`
	KeepRecent       int `json:"keep_recent"`
}

func NewContextBudget(maxTokens int) *ContextBudget {
	if maxTokens <= 0 {
		maxTokens = 128000
	}
	return &ContextBudget{
		MaxTokens:           maxTokens,
		CompactionThreshold: int(float64(maxTokens) * 0.80),
		KeepRecent:          6,
	}
}

// EstimateTokens calculates a fast approximation of tokens for a list of messages.
func (b *ContextBudget) EstimateTokens(messages []Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += len(m.Content) + len(m.Reasoning)
		for _, part := range m.Parts {
			totalChars += len(part.Text)
		}
	}
	// Approximate 1 token ~= 4 characters in English/Code
	tokens := totalChars / 4
	if tokens == 0 && len(messages) > 0 {
		return len(messages) * 10
	}
	return tokens
}

// ShouldCompact checks if the current token estimate exceeds the compaction threshold.
func (b *ContextBudget) ShouldCompact(messages []Message) bool {
	return b.EstimateTokens(messages) >= b.CompactionThreshold
}

// PlanCompaction returns a summary proposal and trimmed message slice.
func (b *ContextBudget) PlanCompaction(messages []Message) (string, []Message) {
	if len(messages) <= b.KeepRecent {
		return "", messages
	}

	cutoff := len(messages) - b.KeepRecent
	oldMessages := messages[:cutoff]
	recentMessages := messages[cutoff:]

	var summaryParts []string
	for _, m := range oldMessages {
		if strings.TrimSpace(m.Content) != "" {
			snippet := m.Content
			if len(snippet) > 80 {
				snippet = snippet[:80] + "..."
			}
			summaryParts = append(summaryParts, m.Role+": "+snippet)
		}
	}

	summary := "Previous context summary:\n" + strings.Join(summaryParts, "\n")
	return summary, recentMessages
}
