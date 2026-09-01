package question

import (
	"context"
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
