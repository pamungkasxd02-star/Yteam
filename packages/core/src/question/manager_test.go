package question

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestQuestionReplyAndSessionBinding(t *testing.T) {
	m := NewManager()
	request, err := m.Ask("ses_a", []schema.QuestionInfo{{Question: "Continue?", Header: "Confirm", Options: []schema.QuestionOption{{Label: "Yes"}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := m.Await(context.Background(), request.ID); done <- err }()
	if err := m.Reply(context.Background(), "ses_other", request.ID, []schema.QuestionAnswer{{"Yes"}}); err != ErrNotFound {
		t.Fatalf("wrong-session reply = %v", err)
	}
	if err := m.Reply(context.Background(), "ses_a", request.ID, []schema.QuestionAnswer{{"Yes"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDurableQuestionReplaysPendingAndTerminalResults(t *testing.T) {
	home := t.TempDir()
	manager, err := OpenManager(home)
	if err != nil {
		t.Fatal(err)
	}
	request, err := manager.Ask("ses_a", []schema.QuestionInfo{{Question: "Continue?"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager, err = OpenManager(home)
	if err != nil {
		t.Fatal(err)
	}
	pending := manager.Pending("ses_a")
	if len(pending) != 1 || pending[0].ID != request.ID {
		t.Fatalf("pending after restart = %#v", pending)
	}
	if err := manager.Reply(context.Background(), "ses_a", request.ID, []schema.QuestionAnswer{{"yes"}}); err != nil {
		t.Fatal(err)
	}
	// A reply can arrive before the consumer calls Await; the terminal result
	// must remain available instead of being dropped with the waiter.
	answers, err := manager.Await(context.Background(), request.ID)
	if err != nil || len(answers) != 1 || answers[0][0] != "yes" {
		t.Fatalf("answers = %#v, err=%v", answers, err)
	}
	manager, err = OpenManager(home)
	if err != nil {
		t.Fatal(err)
	}
	answers, err = manager.Await(context.Background(), request.ID)
	if err != nil || len(answers) != 1 || answers[0][0] != "yes" {
		t.Fatalf("replayed answers = %#v, err=%v", answers, err)
	}
}

func TestDurableQuestionRejectsAndValidatesJournal(t *testing.T) {
	home := t.TempDir()
	manager, err := OpenManager(home)
	if err != nil {
		t.Fatal(err)
	}
	request, err := manager.Ask("ses_a", []schema.QuestionInfo{{Question: "Continue?"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Reject(context.Background(), "ses_a", request.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Await(context.Background(), request.ID); !errors.Is(err, ErrRejected) {
		t.Fatalf("reject error = %v", err)
	}
	if _, err := OpenManager(home); err != nil {
		t.Fatal(err)
	}
	// The configured journal is questions.jsonl; malformed records in that
	// journal must not be silently ignored.
	if err := os.WriteFile(filepath.Join(home, "questions.jsonl"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenManager(home); err == nil {
		t.Fatal("expected malformed journal error")
	}
}
