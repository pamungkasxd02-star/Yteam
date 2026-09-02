package schema

import (
	"encoding/json"
	"testing"
)

func TestMessageMetadataAndPartsRoundTrip(t *testing.T) {
	message := Message{
		Role:         RoleAssistant,
		Content:      "answer",
		Reasoning:    "thinking",
		Model:        "model-x",
		FinishReason: "stop",
		Usage:        &Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		Parts: []MessagePart{
			{Type: "text", Text: "answer"},
			{Type: "reasoning", Text: "thinking"},
		},
	}
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Reasoning != message.Reasoning || decoded.Model != message.Model || decoded.FinishReason != message.FinishReason || decoded.Usage == nil || decoded.Usage.TotalTokens != 5 || len(decoded.Parts) != 2 {
		t.Fatalf("decoded = %#v", decoded)
	}
}
