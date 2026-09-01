package permission

import (
	"context"
	"testing"
	"time"
)

func TestPendingPermissionCanBeAwaitedAndApproved(t *testing.T) {
	engine := New([]Rule{{Action: "edit", Resource: "*", Effect: Ask}})
	request, err := engine.Assert("ses_test", "edit", "file.txt")
	if err != nil || request.ID == "" {
		t.Fatalf("request = %#v, err = %v", request, err)
	}
	done := make(chan error, 1)
	go func() { done <- engine.Await(context.Background(), request.ID) }()
	time.Sleep(time.Millisecond)
	if err := engine.Reply(request.ID, Once); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRejectWakesAwaiter(t *testing.T) {
	engine := New([]Rule{{Action: "bash", Resource: "*", Effect: Ask}})
	request, err := engine.Assert("ses_test", "bash", "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- engine.Await(context.Background(), request.ID) }()
	time.Sleep(time.Millisecond)
	if err := engine.Reply(request.ID, Reject); err == nil {
		t.Fatal("expected reject error")
	}
	if err := <-done; err != ErrRejected {
		t.Fatalf("await error = %v", err)
	}
}
