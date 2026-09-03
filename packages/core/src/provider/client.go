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

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/util"
	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Client struct {
	baseURL, apiKey string
	httpClient      *http.Client
	usage           usageState
	models          Catalog
}

func New(baseURL, apiKey string) *Client {
	client := &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, httpClient: &http.Client{Timeout: 10 * time.Minute}}
	client.models = Catalog{client: client}
	return client
}

func (c *Client) Catalog() *Catalog                    { return &c.models }
func (c *Client) Usage() UsageTotals                   { return c.usage.snapshot() }
func (c *Client) UsageByModel() map[string]UsageTotals { return c.usage.byModel() }
func DefaultFreeModels() []protocol.Model {
	return []protocol.Model{
		{ID: "mimo-v2.5-free", Name: "Mimo v2.5 Free (OpenCode Native)", Description: "OpenCode default free model"},
		{ID: "gemini-2.5-flash", Name: "Google Gemini 2.5 Flash Free", Description: "Fast Google Gemini free tier"},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku (Fast)", Description: "Anthropic fast reasoning model"},
		{ID: "llama-3.3-70b-free", Name: "Llama 3.3 70B Free", Description: "High-capability open model free tier"},
		{ID: "qwen-2.5-coder-32b-free", Name: "Qwen 2.5 Coder 32B Free", Description: "Specialized coding LLM free tier"},
		{ID: "deepseek-v3-free", Name: "DeepSeek V3 Free", Description: "OpenCode Zen free provider"},
	}
}

func (c *Client) Models(ctx context.Context) ([]protocol.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return DefaultFreeModels(), nil
	}
	c.headers(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DefaultFreeModels(), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return DefaultFreeModels(), nil
	}
	var payload struct {
		Data []protocol.Model `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || len(payload.Data) == 0 {
		return DefaultFreeModels(), nil
	}
	return payload.Data, nil
}
func (c *Client) Complete(ctx context.Context, input protocol.ChatRequest, emit func(protocol.StreamDelta) error) error {
	input = c.Catalog().ApplyVariant(input)
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
			Model   string          `json:"model"`
			Usage   *protocol.Usage `json:"usage"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
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
			c.recordUsage(input.Model, payload.Model, payload.Usage)
			return emit(protocol.StreamDelta{Content: payload.Choices[0].Message.Content, ToolCalls: calls, Usage: payload.Usage, Model: payload.Model, FinishReason: payload.Choices[0].FinishReason})
		}
		c.recordUsage(input.Model, payload.Model, payload.Usage)
		if payload.Usage != nil {
			return emit(protocol.StreamDelta{Usage: payload.Usage, Model: payload.Model})
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
			Model   string          `json:"model"`
			Usage   *protocol.Usage `json:"usage"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Delta        struct {
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
		if json.Unmarshal([]byte(value), &chunk) != nil {
			continue
		}
		if chunk.Usage != nil {
			c.recordUsage(input.Model, chunk.Model, chunk.Usage)
		}
		if len(chunk.Choices) == 0 {
			if chunk.Usage != nil {
				if err := emit(protocol.StreamDelta{Usage: chunk.Usage, Model: chunk.Model}); err != nil {
					return err
				}
			}
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
		if delta.Content != "" || delta.Reasoning != "" || chunk.Choices[0].FinishReason != "" {
			if err := emit(protocol.StreamDelta{Content: delta.Content, Reasoning: delta.Reasoning, Model: chunk.Model, FinishReason: chunk.Choices[0].FinishReason}); err != nil {
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

func (c *Client) recordUsage(requestModel, responseModel string, usage *protocol.Usage) {
	model := responseModel
	if model == "" {
		model = requestModel
	}
	definition, _ := c.Catalog().FindCached(model)
	c.usage.add(model, usage, definition)
}

func (c *Client) CompleteRetry(ctx context.Context, input protocol.ChatRequest, emit func(protocol.StreamDelta) error) error {
	return util.Retry(ctx, func() error { return c.Complete(ctx, input, emit) }, util.RetryOptions{})
}

func (c *Client) CompleteRetryWithStatus(ctx context.Context, input protocol.ChatRequest, emit func(protocol.StreamDelta) error, onRetry func(int, error)) error {
	return util.Retry(ctx, func() error { return c.Complete(ctx, input, emit) }, util.RetryOptions{OnRetry: onRetry})
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
