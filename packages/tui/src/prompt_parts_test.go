package tui

import (
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestLongPasteUsesVirtualTextAndExpandsOnSubmit(t *testing.T) {
	value := "line one\nline two\nline three"
	virtual, summarized := pastedVirtualText(value)
	if !summarized || virtual != "[Pasted ~3 lines]" {
		t.Fatalf("virtual=%q summarized=%v", virtual, summarized)
	}
	parts := []schema.MessagePart{{
		Type:   "text",
		Text:   value,
		Source: &schema.PromptPartSource{Text: &schema.PromptTextSource{Start: 0, End: len([]rune(virtual)), Value: virtual}},
	}}
	if got := expandTrackedPastedText(virtual+" ", parts); got != value+" " {
		t.Fatalf("expanded=%q", got)
	}
}

func TestShortPasteKeepsOriginalText(t *testing.T) {
	value := "hello\r\nworld"
	virtual, summarized := pastedVirtualText(value)
	if summarized || virtual != "hello\nworld" {
		t.Fatalf("virtual=%q summarized=%v", virtual, summarized)
	}
}
