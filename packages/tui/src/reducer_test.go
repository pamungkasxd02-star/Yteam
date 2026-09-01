package tui

import (
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestTranscriptReducerAppliesLiveEvents(t *testing.T) {
	reducer := NewTranscriptReducer()
	reducer.Apply(schema.Event{Type: schema.EventPromptAdmitted, Data: map[string]any{"content": "read note"}})
	reducer.Apply(schema.Event{Type: schema.EventTextDelta, Data: map[string]any{"content": "hello"}})
	reducer.Apply(schema.Event{Type: schema.EventToolStarted, Data: map[string]any{"name": "read"}})
	reducer.Apply(schema.Event{Type: schema.EventToolFinished, Data: map[string]any{"name": "read"}})
	if reducer.Status != "running" || len(reducer.Messages) != 3 {
		t.Fatalf("state = %#v", reducer)
	}
	if reducer.Messages[1].Content != "hello" || reducer.Messages[2].ToolState != "done" {
		t.Fatalf("messages = %#v", reducer.Messages)
	}
}
