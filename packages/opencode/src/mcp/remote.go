package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const MaxListPages = 1000

type RemoteConfig struct {
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

type Remote struct {
	cfg    RemoteConfig
	client *http.Client
	mu     sync.Mutex
	next   int64
}
type page struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

func NewRemote(cfg RemoteConfig) (*Remote, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, errors.New("MCP remote URL is empty")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Remote{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

func (r *Remote) Call(ctx context.Context, method string, params any, result any) error {
	r.mu.Lock()
	r.next++
	id := r.next
	r.mu.Unlock()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	for _, transport := range []string{"streamable-http", "sse"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		for key, value := range r.cfg.Headers {
			request.Header.Set(key, value)
		}
		if transport == "sse" {
			request.Header.Set("Accept", "text/event-stream")
		} else {
			request.Header.Set("Accept", "application/json, text/event-stream")
		}
		response, err := r.client.Do(request)
		if err != nil {
			continue
		}
		data, readErr := readRemoteResponse(response)
		_ = response.Body.Close()
		if readErr != nil {
			continue
		}
		if err := decodeRPC(data, result); err == nil {
			return nil
		}
	}
	return fmt.Errorf("MCP remote request failed: %s", method)
}

func (r *Remote) Initialize(ctx context.Context) error {
	var result map[string]any
	return r.Call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "yteam", "version": "0.1.0"},
	}, &result)
}

func (r *Remote) Close() error { return nil }

func (r *Remote) ListTools(ctx context.Context, cursor string) ([]Tool, string, error) {
	params := map[string]any{}
	if cursor != "" {
		params["cursor"] = cursor
	}
	var result page
	if err := r.Call(ctx, "tools/list", params, &result); err != nil {
		return nil, "", err
	}
	return result.Tools, result.NextCursor, nil
}

func (r *Remote) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
			Data string `json:"data,omitempty"`
		} `json:"content"`
		Structured map[string]any `json:"structuredContent,omitempty"`
		IsError    bool           `json:"isError,omitempty"`
	}
	if err := r.Call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result); err != nil {
		return "", err
	}
	var out strings.Builder
	for _, item := range result.Content {
		if item.Text != "" {
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(item.Text)
		}
		if item.Data != "" {
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(item.Data)
		}
	}
	if out.Len() == 0 && result.Structured != nil {
		data, _ := json.Marshal(result.Structured)
		out.Write(data)
	}
	if result.IsError {
		return out.String(), errors.New(out.String())
	}
	return out.String(), nil
}

func (r *Remote) AllTools(ctx context.Context) ([]Tool, error) {
	var result []Tool
	seen := map[string]bool{}
	cursor := ""
	for count := 0; count < MaxListPages; count++ {
		items, next, err := r.ListTools(ctx, cursor)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if next == "" {
			return result, nil
		}
		if seen[next] {
			return nil, fmt.Errorf("MCP list returned duplicate cursor: %s", next)
		}
		seen[next] = true
		cursor = next
	}
	return nil, fmt.Errorf("MCP list exceeded %d pages", MaxListPages)
}

func readRemoteResponse(response *http.Response) ([]byte, error) {
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %s", response.Status)
	}
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "data:") {
				return []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), nil
			}
		}
		return nil, scanner.Err()
	}
	return io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
}
func decodeRPC(data []byte, result any) error {
	var response struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("MCP %d: %s", response.Error.Code, response.Error.Message)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(response.Result, result)
}
