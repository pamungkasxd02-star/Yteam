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

func (p *Picker) Filtered() []PickerItem {
	query := strings.ToLower(strings.TrimSpace(p.Query))
	if query == "" {
		return append([]PickerItem(nil), p.Items...)
	}
	result := make([]PickerItem, 0, len(p.Items))
	for _, item := range p.Items {
		value := strings.ToLower(item.ID + " " + item.Label + " " + item.Description)
		if strings.Contains(value, query) {
			result = append(result, item)
		}
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
