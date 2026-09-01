package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Client struct {
	baseURL, apiKey string
	httpClient      *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, httpClient: &http.Client{Timeout: 10 * time.Minute}}
}
func (c *Client) Models(ctx context.Context) ([]protocol.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.headers(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, apiError(resp)
	}
	var payload struct {
		Data []protocol.Model `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}
func (c *Client) Complete(ctx context.Context, input protocol.ChatRequest, emit func(protocol.StreamDelta) error) error {
	input.Stream = true
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.headers(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return apiError(resp)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		var payload struct {
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return err
		}
		if len(payload.Choices) > 0 {
			calls := make([]schema.ToolCall, 0, len(payload.Choices[0].Message.ToolCalls))
			for _, call := range payload.Choices[0].Message.ToolCalls {
				calls = append(calls, schema.ToolCall{ID: call.ID, Type: call.Type, Name: call.Function.Name, Arguments: call.Function.Arguments})
			}
			return emit(protocol.StreamDelta{Content: payload.Choices[0].Message.Content, ToolCalls: calls})
		}
		return nil
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	toolCalls := map[int]*schema.ToolCall{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if value == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(value), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		for _, call := range delta.ToolCalls {
			current := toolCalls[call.Index]
			if current == nil {
				current = &schema.ToolCall{ID: call.ID, Type: call.Type}
				toolCalls[call.Index] = current
			}
			if call.ID != "" {
				current.ID = call.ID
			}
			if call.Type != "" {
				current.Type = call.Type
			}
			if call.Function.Name != "" {
				current.Name += call.Function.Name
			}
			current.Arguments += call.Function.Arguments
		}
		if delta.Content != "" || delta.Reasoning != "" {
			if err := emit(protocol.StreamDelta{Content: delta.Content, Reasoning: delta.Reasoning}); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(toolCalls) > 0 {
		indices := make([]int, 0, len(toolCalls))
		for index := range toolCalls {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		calls := make([]schema.ToolCall, 0, len(indices))
		for _, index := range indices {
			calls = append(calls, *toolCalls[index])
		}
		return emit(protocol.StreamDelta{ToolCalls: calls})
	}
	return nil
}
func (c *Client) headers(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else if strings.Contains(c.baseURL, "opencode.ai/zen") {
		req.Header.Set("Authorization", "Bearer public")
	}
}
func apiError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	text := strings.TrimSpace(string(data))
	if text == "" {
		text = resp.Status
	}
	return fmt.Errorf("provider %s: %s", resp.Status, text)
}
