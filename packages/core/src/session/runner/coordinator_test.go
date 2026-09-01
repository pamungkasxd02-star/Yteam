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
