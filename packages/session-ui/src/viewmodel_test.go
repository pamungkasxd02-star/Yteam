package sessionui

import (
	"testing"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestSessionUIViewModel(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	s := session.Session{
		ID:        "ses_123",
		Title:     "Test Session",
		Directory: "/project",
		Messages: []session.Message{
			{ID: "msg_1", Role: "user", Content: "hello", CreatedAt: now},
			{ID: "msg_2", Role: "assistant", Content: "world", CreatedAt: now},
		},
	}

	vm := FromSession(s)
	if vm.ID != "ses_123" || vm.Title != "Test Session" || len(vm.MessageList) != 2 {
		t.Fatalf("unexpected viewmodel: %#v", vm)
	}
	if vm.MessageList[0].Author != "You" || vm.MessageList[1].Author != "Agent" {
		t.Fatalf("unexpected authors: %s, %s", vm.MessageList[0].Author, vm.MessageList[1].Author)
	}
}
