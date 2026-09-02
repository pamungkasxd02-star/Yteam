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
	cfg          RemoteConfig
	client       *http.Client
	mu           sync.Mutex
	next         int64
	capabilities map[string]any
}
type page struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor"`
}

type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []map[string]any `json:"arguments,omitempty"`
}

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
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
	if err := r.Call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "yteam", "version": "0.1.0"},
	}, &result); err != nil {
		return err
	}
	r.mu.Lock()
	if value, ok := result["capabilities"].(map[string]any); ok {
		r.capabilities = value
	} else {
		r.capabilities = map[string]any{}
	}
	r.mu.Unlock()
	return nil
}

func (r *Remote) Close() error { return nil }

func (r *Remote) ListTools(ctx context.Context, cursor string) ([]Tool, string, error) {
	items, next, err := r.listToolsPage(ctx, cursor)
	if next == nil {
		return items, "", err
	}
	return items, *next, err
}

func (r *Remote) listToolsPage(ctx context.Context, cursor string) ([]Tool, *string, error) {
	params := map[string]any{}
	if cursor != "" {
		params["cursor"] = cursor
	}
	var result page
	if err := r.Call(ctx, "tools/list", params, &result); err != nil {
		return nil, nil, err
	}
	return result.Tools, result.NextCursor, nil
}

func (r *Remote) Supports(capability string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.capabilities[capability]
	return ok
}

func (r *Remote) ListPrompts(ctx context.Context, cursor string) ([]Prompt, string, error) {
	if !r.Supports("prompts") {
		return []Prompt{}, "", nil
	}
	items, next, err := r.listPromptsPage(ctx, cursor)
	return items, cursorString(next), err
}

func (r *Remote) ListResources(ctx context.Context, cursor string) ([]Resource, string, error) {
	if !r.Supports("resources") {
		return []Resource{}, "", nil
	}
	items, next, err := r.listResourcesPage(ctx, cursor)
	return items, cursorString(next), err
}

func (r *Remote) ListResourceTemplates(ctx context.Context, cursor string) ([]ResourceTemplate, string, error) {
	if !r.Supports("resources") {
		return []ResourceTemplate{}, "", nil
	}
	items, next, err := r.listResourceTemplatesPage(ctx, cursor)
	return items, cursorString(next), err
}

func (r *Remote) AllPrompts(ctx context.Context) ([]Prompt, error) {
	return paginateCatalogPages(ctx, func(cursor string) ([]Prompt, *string, error) { return r.listPromptsPage(ctx, cursor) })
}

func (r *Remote) AllResources(ctx context.Context) ([]Resource, error) {
	return paginateCatalogPages(ctx, func(cursor string) ([]Resource, *string, error) { return r.listResourcesPage(ctx, cursor) })
}

func (r *Remote) AllResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return paginateCatalogPages(ctx, func(cursor string) ([]ResourceTemplate, *string, error) {
		return r.listResourceTemplatesPage(ctx, cursor)
	})
}

func (r *Remote) listPromptsPage(ctx context.Context, cursor string) ([]Prompt, *string, error) {
	return listCatalogPage[Prompt](ctx, r, "prompts/list", "prompts", cursor)
}

func (r *Remote) listResourcesPage(ctx context.Context, cursor string) ([]Resource, *string, error) {
	return listCatalogPage[Resource](ctx, r, "resources/list", "resources", cursor)
}

func (r *Remote) listResourceTemplatesPage(ctx context.Context, cursor string) ([]ResourceTemplate, *string, error) {
	return listCatalogPage[ResourceTemplate](ctx, r, "resources/templates/list", "resourceTemplates", cursor)
}

func listCatalogPage[T any](ctx context.Context, remote *Remote, method, field, cursor string) ([]T, *string, error) {
	params := map[string]any{}
	if cursor != "" {
		params["cursor"] = cursor
	}
	var result map[string]json.RawMessage
	if err := remote.Call(ctx, method, params, &result); err != nil {
		return nil, nil, err
	}
	var items []T
	if raw, ok := result[field]; ok {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, nil, err
		}
	}
	var next *string
	if raw, ok := result["nextCursor"]; ok && string(raw) != "null" {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, nil, err
		}
		next = &value
	}
	return items, next, nil
}

func cursorString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func paginateCatalogPages[T any](ctx context.Context, list func(string) ([]T, *string, error)) ([]T, error) {
	items := []T{}
	seen := map[string]bool{}
	cursor := ""
	for count := 0; count < MaxListPages; count++ {
		page, next, err := list(cursor)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if next == nil {
			return items, nil
		}
		if seen[*next] {
			return nil, fmt.Errorf("MCP list returned duplicate cursor: %s", *next)
		}
		seen[*next] = true
		cursor = *next
	}
	return nil, fmt.Errorf("MCP list exceeded %d pages", MaxListPages)
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
		items, next, err := r.listToolsPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
		if next == nil {
			return result, nil
		}
		if seen[*next] {
			return nil, fmt.Errorf("MCP list returned duplicate cursor: %s", *next)
		}
		seen[*next] = true
		cursor = *next
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
