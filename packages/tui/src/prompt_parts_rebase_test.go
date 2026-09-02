package tui

import (
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestRebasePromptPartsMovesRangesForEditsBeforeMarkers(t *testing.T) {
	parts := []schema.MessagePart{{Type: "text", Text: "expanded", Source: &schema.PromptPartSource{Text: &schema.PromptTextSource{Start: 5, End: 10, Value: "[P]"}}}}
	got := rebasePromptParts(parts, 0, 0, "abc")
	if len(got) != 1 || got[0].Source.Text.Start != 8 || got[0].Source.Text.End != 13 {
		t.Fatalf("rebased parts = %#v", got)
	}
}

func TestRebasePromptPartsRemovesMarkerWhenEditOverlapsIt(t *testing.T) {
	parts := []schema.MessagePart{{Type: "file", Source: &schema.PromptPartSource{Start: 2, End: 8, Value: "@note"}}}
	if got := rebasePromptParts(parts, 4, 5, ""); len(got) != 0 {
		t.Fatalf("overlapping parts = %#v", got)
	}
}

func TestRebasePromptPartsPreservesUnrelatedParts(t *testing.T) {
	parts := []schema.MessagePart{{Type: "file", Source: &schema.PromptPartSource{Start: 2, End: 8, Value: "@note"}}}
	got := rebasePromptParts(parts, 12, 12, "x")
	if len(got) != 1 || got[0].Source.Start != 2 || got[0].Source.End != 8 {
		t.Fatalf("unrelated parts = %#v", got)
	}
}
