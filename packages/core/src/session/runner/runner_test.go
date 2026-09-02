package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/tool"
	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestRunnerContinuesAfterToolCall(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("from tool"), 0o600); err != nil {
		t.Fatal(err)
	}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/event-stream")
		if hits == 1 {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"note.txt\\\"}\"}}]}}]}\n\n"))
		} else {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"sudah dibaca\"}}]}\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	store, err := session.Open(t.TempDir(), root)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	permissions := permission.New([]permission.Rule{{Action: "read", Resource: "*", Effect: permission.Allow}})
	runner := &Runner{Provider: provider.New(server.URL, ""), Store: store, Tools: tool.ReadOnly(permissions), MaxSteps: 2}
	if err := runner.Run(context.Background(), sess, "test-model", ""); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("provider turns = %d, want 2", hits)
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("messages = %#v", sess.Messages)
	}
	if sess.Messages[1].Role != "tool" || !strings.Contains(sess.Messages[1].Content, "from tool") {
		t.Fatalf("tool message = %#v", sess.Messages[1])
	}
	if sess.Messages[2].Content != "sudah dibaca" {
		t.Fatalf("assistant = %#v", sess.Messages[2])
	}
}

func TestRunnerPersistsProviderMetadataAndParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"model\":\"test-model\",\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"model\":\"test-model\",\"choices\":[{\"finish_reason\":\"stop\",\"delta\":{\"content\":\"answer\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"model\":\"test-model\",\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6},\"choices\":[]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	root := t.TempDir()
	store, err := session.Open(t.TempDir(), root)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Provider: provider.New(server.URL, ""), Store: store, MaxSteps: 1}
	if err := r.Run(context.Background(), sess, "test-model", ""); err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("messages = %#v", sess.Messages)
	}
	message := sess.Messages[0]
	if message.Reasoning != "think" || message.Model != "test-model" || message.FinishReason != "stop" || message.Usage == nil || message.Usage.TotalTokens != 6 {
		t.Fatalf("metadata = %#v", message)
	}
	if len(message.Parts) != 2 || message.Parts[0].Type != "text" || message.Parts[1].Type != "reasoning" {
		t.Fatalf("parts = %#v", message.Parts)
	}
	reloaded, err := store.Load(sess.ID)
	if err != nil || reloaded.Messages[0].Usage == nil || reloaded.Messages[0].Parts[1].Text != "think" {
		t.Fatalf("reloaded = %#v, err=%v", reloaded.Messages, err)
	}
}

var _ = protocol.ChatRequest{}
var _ = schema.RoleUser
