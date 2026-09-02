package permission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestDurablePermissionReplaysPendingAndAlwaysRules(t *testing.T) {
	home := t.TempDir()
	rules := []Rule{{Action: "edit", Resource: "*", Effect: Ask}}
	engine, err := Open(home, rules)
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.Assert("ses_test", "edit", "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	engine, err = Open(home, rules)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := engine.Get(request.ID); !ok {
		t.Fatal("pending request was not replayed")
	}
	if err := engine.Reply(request.ID, Always); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate("edit", "file.txt"); got != Allow {
		t.Fatalf("always evaluation = %q", got)
	}
	engine, err = Open(home, rules)
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate("edit", "file.txt"); got != Allow {
		t.Fatalf("replayed always evaluation = %q", got)
	}
}

func TestPermissionReplyBeforeAwaitSurvivesRestart(t *testing.T) {
	home := t.TempDir()
	engine, err := Open(home, []Rule{{Action: "bash", Resource: "*", Effect: Ask}})
	if err != nil {
		t.Fatal(err)
	}
	request, err := engine.Assert("ses_test", "bash", "echo hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Reply(request.ID, Once); err != nil {
		t.Fatal(err)
	}
	engine, err = Open(home, []Rule{{Action: "bash", Resource: "*", Effect: Ask}})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Await(context.Background(), request.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "permissions.jsonl"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(home, nil); err == nil {
		t.Fatal("expected malformed permission journal error")
	}
	if !errors.Is(ErrRejected, ErrRejected) {
		t.Fatal("sentinel check failed")
	}
}
