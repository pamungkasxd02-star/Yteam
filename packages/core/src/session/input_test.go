package session

import (
	"context"
	"testing"
)

func TestInputQueuePromotesSteerAndOneQueuedInput(t *testing.T) {
	queue := NewInputQueue()
	first, err := queue.Admit("ses_test", "queued", DeliveryQueue)
	if err != nil {
		t.Fatal(err)
	}
	steer, err := queue.Admit("ses_test", "steer", DeliverySteer)
	if err != nil {
		t.Fatal(err)
	}
	second, err := queue.Admit("ses_test", "queued two", DeliveryQueue)
	if err != nil {
		t.Fatal(err)
	}
	items, err := queue.Promote("ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != steer.ID || items[1].ID != first.ID {
		t.Fatalf("promoted = %#v", items)
	}
	items, err = queue.Promote("ses_test")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("second promotion = %#v", items)
	}
	if pending := queue.Pending("ses_test"); len(pending) != 0 {
		t.Fatalf("pending after promotion = %#v", pending)
	}
}

func TestInputQueueWaitAndInterrupt(t *testing.T) {
	queue := NewInputQueue()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queue.Interrupt("ses_test")
	if !queue.IsInterrupted("ses_test") {
		t.Fatal("interrupt was not recorded")
	}
	queue.ClearInterrupt("ses_test")
	go func() { _, _ = queue.Admit("ses_test", "wake", DeliveryQueue) }()
	if err := queue.Wait(ctx, "ses_test"); err != nil {
		t.Fatal(err)
	}
}

func TestInputQueueReplaysAdmissionAndPromotionAfterRestart(t *testing.T) {
	home := t.TempDir()
	first, err := OpenInputQueue(home)
	if err != nil {
		t.Fatal(err)
	}
	item, err := first.Admit("ses_restart", "persist me", DeliverySteer)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := first.PromoteByID(item.ID); err != nil || !ok {
		t.Fatal("promotion failed")
	}
	second, err := OpenInputQueue(home)
	if err != nil {
		t.Fatal(err)
	}
	if pending := second.Pending("ses_restart"); len(pending) != 0 {
		t.Fatalf("pending after replay = %#v", pending)
	}
	if items, err := second.Promote("ses_restart"); err != nil || len(items) != 0 {
		t.Fatalf("re-promoted = %#v", items)
	}
}
