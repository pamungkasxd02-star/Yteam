package runner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCoordinatorJoinsSameSession(t *testing.T) {
	c := NewCoordinator()
	started := make(chan struct{})
	release := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	var first, second error
	go func() {
		defer group.Done()
		first = c.Run(context.Background(), "ses_same", func(context.Context) error {
			close(started)
			<-release
			return errors.New("finished")
		})
	}()
	<-started
	go func() {
		defer group.Done()
		second = c.Run(context.Background(), "ses_same", func(context.Context) error {
			t.Error("same session started twice")
			return nil
		})
	}()
	time.Sleep(10 * time.Millisecond)
	if len(c.Active()) != 1 {
		t.Fatalf("active = %#v", c.Active())
	}
	close(release)
	group.Wait()
	if first == nil || second == nil || first.Error() != second.Error() {
		t.Fatalf("errors = %v, %v", first, second)
	}
}

func TestCoordinatorWakeCoalescesAndRunsSuccessor(t *testing.T) {
	c := NewCoordinator()
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	runs := 0
	work := func(context.Context) error {
		mu.Lock()
		runs++
		current := runs
		mu.Unlock()
		if current == 1 {
			close(started)
			<-release
		}
		return nil
	}
	c.Wake("ses_wake", work)
	<-started
	c.Wake("ses_wake", work)
	c.Wake("ses_wake", work)
	close(release)
	deadline := time.After(time.Second)
	for {
		mu.Lock()
		count := runs
		mu.Unlock()
		if count == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("runs = %d", count)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestCoordinatorInterruptWaitsForCleanup(t *testing.T) {
	c := NewCoordinator()
	started := make(chan struct{})
	c.Wake("ses_interrupt", func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() })
	<-started
	if err := c.Interrupt(context.Background(), "ses_interrupt"); err != nil {
		t.Fatal(err)
	}
	if len(c.Active()) != 0 {
		t.Fatalf("active after interrupt = %#v", c.Active())
	}
}
