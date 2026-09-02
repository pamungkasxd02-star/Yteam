package provider

import (
	"sync"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

type UsageTotals struct {
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCost    float64 `json:"estimated_cost"`
}

type usageState struct {
	mu    sync.RWMutex
	total UsageTotals
	model map[string]UsageTotals
}

func (u *usageState) add(model string, usage *protocol.Usage, definition protocol.Model) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.total.Requests++
	if usage == nil {
		return
	}
	prompt, completion, total := usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens
	if total == 0 {
		total = prompt + completion
	}
	cost := float64(prompt)*definition.InputCostPerToken + float64(completion)*definition.OutputCostPerToken
	u.total.PromptTokens += prompt
	u.total.CompletionTokens += completion
	u.total.TotalTokens += total
	u.total.EstimatedCost += cost
	if u.model == nil {
		u.model = map[string]UsageTotals{}
	}
	item := u.model[model]
	item.Requests++
	item.PromptTokens += prompt
	item.CompletionTokens += completion
	item.TotalTokens += total
	item.EstimatedCost += cost
	u.model[model] = item
}

func (u *usageState) snapshot() UsageTotals {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.total
}

func (u *usageState) byModel() map[string]UsageTotals {
	u.mu.RLock()
	defer u.mu.RUnlock()
	result := make(map[string]UsageTotals, len(u.model))
	for key, value := range u.model {
		result[key] = value
	}
	return result
}
