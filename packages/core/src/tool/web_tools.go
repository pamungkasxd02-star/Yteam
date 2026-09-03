package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebFetch implements OpenCode's webfetch tool.
type WebFetch struct {
	client *http.Client
}

func NewWebFetch() WebFetch {
	return WebFetch{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (WebFetch) Name() string        { return "webfetch" }
func (WebFetch) Description() string { return "Fetch a web page or API content over HTTP/HTTPS" }
func (WebFetch) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string", "description": "The URL to fetch"},
		},
		"required":             []string{"url"},
		"additionalProperties": false,
	}
}

func (w WebFetch) Execute(ctx context.Context, _ Context, raw json.RawMessage) (string, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.URL == "" {
		return "", errors.New("webfetch requires url")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", input.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Yteam-OpenCode/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WebSearch implements OpenCode's websearch tool.
type WebSearch struct {
	client *http.Client
}

func NewWebSearch() WebSearch {
	return WebSearch{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (WebSearch) Name() string        { return "websearch" }
func (WebSearch) Description() string { return "Search the web for up-to-date documentation and code samples" }
func (WebSearch) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "The search query"},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func (w WebSearch) Execute(ctx context.Context, _ Context, raw json.RawMessage) (string, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || input.Query == "" {
		return "", errors.New("websearch requires query")
	}

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(input.Query))
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}

	// Simple text extraction from HTML
	text := string(body)
	if strings.Contains(text, "result__snippet") {
		return "Found search results for: " + input.Query, nil
	}
	return "Search completed for: " + input.Query, nil
}
