package session

import (
	"strings"
	"testing"
)

func TestCompactMessagesKeepsRecentAndCapsToolOutput(t *testing.T) {
	store, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 5; index++ {
		if err := store.Append(sess.ID, Message{Role: "user", Content: "message"}); err != nil {
			t.Fatal(err)
		}
	}
	longOutput := strings.Repeat("x", MaxToolOutput+100)
	if err := store.Append(sess.ID, Message{Role: "tool", Content: longOutput}); err != nil {
		t.Fatal(err)
	}
	compaction, err := store.CompactMessages(sess.ID, "Keep the objective.", 2)
	if err != nil {
		t.Fatal(err)
	}
	if compaction.Summary != "Keep the objective." || len(compaction.Recent) != 2 {
		t.Fatalf("compaction = %#v", compaction)
	}
	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 3 || !strings.Contains(loaded.Messages[0].Content, "Keep the objective.") {
		t.Fatalf("loaded = %#v", loaded.Messages)
	}
	if len(loaded.Messages[2].Content) > MaxToolOutput+20 {
		t.Fatalf("tool output was not capped: %d", len(loaded.Messages[2].Content))
	}
}
