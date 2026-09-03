package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

// AnthropicAdapter handles native Claude Messages API streaming.
type AnthropicAdapter struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewAnthropicAdapter(baseURL, apiKey string) *AnthropicAdapter {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (a *AnthropicAdapter) Name() string        { return "Anthropic" }
func (a *AnthropicAdapter) Type() ProviderType { return ProviderAnthropic }

func (a *AnthropicAdapter) Models(ctx context.Context) ([]protocol.Model, error) {
	return []protocol.Model{
		{ID: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet"},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku"},
	}, nil
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
}

func (a *AnthropicAdapter) Complete(ctx context.Context, req protocol.ChatRequest, emit func(StreamDelta) error) error {
	var system string
	var messages []anthropicMessage

	for _, msg := range req.Messages {
		if string(msg.Role) == "system" {
			system = msg.Content
		} else {
			messages = append(messages, anthropicMessage{Role: string(msg.Role), Content: msg.Content})
		}
	}

	modelName := strings.TrimPrefix(req.Model, "anthropic/")
	if modelName == "" {
		modelName = "claude-3-7-sonnet-20250219"
	}

	bodyData, err := json.Marshal(anthropicRequest{
		Model:     modelName,
		Messages:  messages,
		System:    system,
		MaxTokens: 4096,
		Stream:    true,
	})
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/messages", bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anthropic error (%d): %s", resp.StatusCode, string(out))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")
		if raw == "[DONE]" {
			break
		}

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(raw), &event); err == nil {
			if event.Delta.Text != "" {
				_ = emit(StreamDelta{
					Content: event.Delta.Text,
				})
			}
		}
	}
	return scanner.Err()
}
