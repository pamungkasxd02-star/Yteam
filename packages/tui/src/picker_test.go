package tui

import "testing"

func TestPickerFiltersAndWraps(t *testing.T) {
	picker := NewPicker("Model", []PickerItem{
		{ID: "alpha", Label: "Alpha"},
		{ID: "beta", Label: "Beta"},
		{ID: "gamma", Label: "Gamma"},
	})
	picker.Move(-1)
	item, ok := picker.Selected()
	if !ok || item.ID != "gamma" {
		t.Fatalf("selected = %#v, %v", item, ok)
	}
	picker.SetQuery("be")
	item, ok = picker.Selected()
	if !ok || item.ID != "beta" {
		t.Fatalf("filtered = %#v, %v", item, ok)
	}
	picker.Move(1)
	item, ok = picker.Selected()
	if !ok || item.ID != "beta" {
		t.Fatalf("single-item wrap = %#v, %v", item, ok)
	}
}

func TestPickerSelectsByNumberAfterFilter(t *testing.T) {
	picker := NewPicker("Agent", []PickerItem{
		{ID: "build", Label: "build", Description: "Implement"},
		{ID: "plan", Label: "plan", Description: "Inspect"},
	})
	picker.SetQuery("inspect")
	item, ok := picker.Selected()
	if !ok || item.ID != "plan" {
		t.Fatalf("item = %#v, %v", item, ok)
	}
	if atoi("2") != 2 || atoi("abc") != 0 || atoi("12x") != 0 {
		t.Fatal("invalid numeric parsing")
	}
}

func TestPickerSupportsPageHomeAndEndNavigation(t *testing.T) {
	items := make([]PickerItem, 12)
	for index := range items {
		items[index] = PickerItem{ID: "item-" + string(rune('a'+index)), Label: "item"}
	}
	picker := NewPicker("Commands", items)
	picker.Page(1)
	if picker.Index != 5 {
		t.Fatalf("page down index = %d", picker.Index)
	}
	picker.End()
	if picker.Index != 11 {
		t.Fatalf("end index = %d", picker.Index)
	}
	picker.Page(-1)
	if picker.Index != 6 {
		t.Fatalf("page up index = %d", picker.Index)
	}
	picker.Home()
	if picker.Index != 0 {
		t.Fatalf("home index = %d", picker.Index)
	}
}
