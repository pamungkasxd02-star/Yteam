package session

import "testing"

func TestRunStateIsDurableAndResetsForNextRun(t *testing.T) {
	store, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	busy, err := store.SetRunState(sess.ID, RunBusy, 0, "")
	if err != nil || busy.RunStatus != RunBusy || busy.RunStartedAt == "" || busy.RunFinishedAt != "" {
		t.Fatalf("busy = %#v, err=%v", busy, err)
	}
	failed, err := store.SetRunState(sess.ID, RunFailed, 1, "provider failed")
	if err != nil || failed.RunError != "provider failed" || failed.RunFinishedAt == "" {
		t.Fatalf("failed = %#v, err=%v", failed, err)
	}
	next, err := store.SetRunState(sess.ID, RunBusy, 0, "")
	if err != nil || next.RunError != "" || next.RunFinishedAt != "" || next.RunStartedAt == "" {
		t.Fatalf("next = %#v, err=%v", next, err)
	}
	reloaded, err := store.Load(sess.ID)
	if err != nil || reloaded.RunStatus != RunBusy {
		t.Fatalf("reloaded = %#v, err=%v", reloaded, err)
	}
}
