package session

import "testing"

func TestLifecycleRenameForkExportAndDelete(t *testing.T) {
	store, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(sess.ID, Message{Role: "user", Content: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename(sess.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	fork, err := store.Fork(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fork.Messages) != 1 || fork.Messages[0].Content != "first" {
		t.Fatalf("fork = %#v", fork.Messages)
	}
	md, err := store.ExportMarkdown(fork.ID)
	if err != nil || md == "" {
		t.Fatalf("markdown = %q, err = %v", md, err)
	}
	jsonData, err := store.ExportJSON(fork.ID)
	if err != nil || !contains(string(jsonData), `"messages"`) {
		t.Fatalf("json export = %s, err = %v", jsonData, err)
	}
	if err := store.Delete(fork.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(fork.ID); err == nil {
		t.Fatal("deleted session still loads")
	}
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
