package event

import (
	"context"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestJournalPublishesAndResumesSequence(t *testing.T) {
	home := t.TempDir()
	j, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := j.Subscribe(ctx)
	first, err := j.Publish(context.Background(), schema.EventSessionCreated, "ses_test", map[string]any{"title": "one"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || first.ID == "" {
		t.Fatalf("event = %#v", first)
	}
	got := <-updates
	if got.ID != first.ID {
		t.Fatalf("published event = %#v", got)
	}
	// A fresh journal must continue the aggregate sequence rather than reset it.
	j, err = Open(home)
	if err != nil {
		t.Fatal(err)
	}
	second, err := j.Publish(context.Background(), schema.EventPromptAdmitted, "ses_test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Sequence != 2 {
		t.Fatalf("sequence = %d", second.Sequence)
	}
}
