package runtime

import (
	"context"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/event"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestAdmitInputPersistsAcrossStoreRestart(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	journal, err := event.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	app.AttachEvents(journal)
	item, err := app.AdmitInput(current.ID, "persisted input", session.DeliverySteer)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || len(app.PendingInputs(current.ID)) != 1 {
		t.Fatalf("item = %#v", item)
	}
	reloaded, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	if pending := reloaded.Inputs().Pending(current.ID); len(pending) != 1 || pending[0].ID != item.ID {
		t.Fatalf("reloaded = %#v", pending)
	}
	if events, err := journal.Since(current.ID, 0); err != nil || len(events) != 1 || events[0].Type != "session.prompt.admitted" {
		t.Fatalf("events = %#v, err = %v", events, err)
	}
	_ = context.Background()
}
