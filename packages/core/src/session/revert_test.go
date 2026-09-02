package session

import "testing"

func TestRevertStateStagesClearsCommitsAndPersists(t *testing.T) {
	store, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(sess.ID, Message{Role: "user", Content: "change"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	messageID := loaded.Messages[0].ID
	staged, err := store.StageRevert(sess.ID, messageID, "diff --git a/file b/file")
	if err != nil {
		t.Fatal(err)
	}
	if staged.Revert == nil || staged.Revert.MessageID != messageID {
		t.Fatalf("staged = %#v", staged.Revert)
	}
	reloaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Revert == nil {
		t.Fatal("revert was not persisted")
	}
	cleared, err := store.ClearRevert(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Revert != nil {
		t.Fatal("revert was not cleared")
	}
	staged, err = store.StageRevert(sess.ID, messageID, "diff")
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitRevert(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Revert != nil {
		t.Fatal("revert was not committed")
	}
	_ = staged
	if _, err := store.StageRevert(sess.ID, "msg_missing", "diff"); err != ErrMessageNotFound {
		t.Fatalf("missing message error = %v", err)
	}
}
