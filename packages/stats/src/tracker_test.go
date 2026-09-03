package stats

import (
	"testing"
)

func TestTrackerRecordAndSnapshot(t *testing.T) {
	home := t.TempDir()
	tracker, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}

	err = tracker.RecordRun(100, 50, 2)
	if err != nil {
		t.Fatal(err)
	}

	snap := tracker.Snapshot()
	if snap.TotalTokens != 150 || snap.RunCount != 1 || snap.ToolInvocations != 2 {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}

	// Reopen and verify persistence
	tracker2, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	snap2 := tracker2.Snapshot()
	if snap2.TotalTokens != 150 || snap2.RunCount != 1 {
		t.Fatalf("persistence failed: %#v", snap2)
	}
}
