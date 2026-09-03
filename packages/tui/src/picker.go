package tui

import "strings"

// Picker is the state machine behind OpenCode-style searchable selection
// dialogs. Rendering and input transport stay separate so the same behavior
// works in a real terminal and in deterministic pipe/tests.
type PickerItem struct {
	ID          string
	Label       string
	Description string
}

type Picker struct {
	Title string
	Items []PickerItem
	Query string
	Index int
}

func NewPicker(title string, items []PickerItem) *Picker {
	copyItems := append([]PickerItem(nil), items...)
	picker := &Picker{Title: title, Items: copyItems}
	picker.clamp()
	return picker
}

func fuzzyMatch(pattern, text string) (bool, int) {
	if pattern == "" {
		return true, 0
	}
	pIdx := 0
	pRunes := []rune(strings.ToLower(pattern))
	tRunes := []rune(strings.ToLower(text))
	score := 0
	consecutive := 0

	for i, tr := range tRunes {
		if pIdx < len(pRunes) && tr == pRunes[pIdx] {
			pIdx++
			score += 10 + (consecutive * 5)
			if i == 0 || tRunes[i-1] == ' ' || tRunes[i-1] == '_' || tRunes[i-1] == '-' || tRunes[i-1] == '/' {
				score += 20
			}
			consecutive++
		} else {
			consecutive = 0
		}
	}
	return pIdx == len(pRunes), score
}

func (p *Picker) Filtered() []PickerItem {
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return append([]PickerItem(nil), p.Items...)
	}
	type scoredItem struct {
		item  PickerItem
		score int
	}
	matches := make([]scoredItem, 0, len(p.Items))
	for _, item := range p.Items {
		target := item.ID + " " + item.Label + " " + item.Description
		matched, score := fuzzyMatch(query, target)
		if matched {
			matches = append(matches, scoredItem{item: item, score: score})
		}
	}
	// Sort highest score first
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].score > matches[i].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
	result := make([]PickerItem, len(matches))
	for i, m := range matches {
		result[i] = m.item
	}
	return result
}

func (p *Picker) Move(delta int) {
	items := p.Filtered()
	if len(items) == 0 {
		p.Index = 0
		return
	}
	p.Index += delta
	if p.Index < 0 {
		p.Index = len(items) - 1
	}
	if p.Index >= len(items) {
		p.Index = 0
	}
}

func (p *Picker) Page(delta int) {
	items := p.Filtered()
	if len(items) == 0 {
		p.Index = 0
		return
	}
	if delta < 0 {
		p.Index -= 5
	} else {
		p.Index += 5
	}
	p.clamp()
}

func (p *Picker) Home() {
	p.Index = 0
	p.clamp()
}

func (p *Picker) End() {
	p.Index = len(p.Filtered()) - 1
	p.clamp()
}

func (p *Picker) SetQuery(query string) { p.Query = query; p.Index = 0; p.clamp() }

func (p *Picker) Selected() (PickerItem, bool) {
	items := p.Filtered()
	if p.Index < 0 || p.Index >= len(items) {
		return PickerItem{}, false
	}
	return items[p.Index], true
}

func (p *Picker) clamp() {
	items := p.Filtered()
	if len(items) == 0 {
		p.Index = 0
		return
	}
	if p.Index >= len(items) {
		p.Index = len(items) - 1
	}
	if p.Index < 0 {
		p.Index = 0
	}
}

func atoi(value string) int {
	if value == "" {
		return 0
	}
	result := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0
		}
		result = result*10 + int(char-'0')
	}
	return result
}
