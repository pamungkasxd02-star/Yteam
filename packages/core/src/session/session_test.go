package session

import "testing"

func TestStoreRoundTripAndList(t *testing.T) {
	projectRoot := t.TempDir()
	store, err := Open(t.TempDir(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Directory != projectRoot {
		t.Fatalf("directory = %q", sess.Directory)
	}
	if err := store.Append(sess.ID, Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "hello" {
		t.Fatalf("messages = %#v", loaded.Messages)
	}
	items, err := store.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %v, %#v", err, items)
	}
}

func TestStoreRejectsPathTraversal(t *testing.T) {
	store, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("..\\escape"); err == nil {
		t.Fatal("expected invalid session ID")
	}
}
