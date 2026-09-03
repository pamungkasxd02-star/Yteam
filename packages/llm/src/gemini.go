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

// GeminiAdapter handles Google Gemini native REST streaming API.
type GeminiAdapter struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewGeminiAdapter(baseURL, apiKey string) *GeminiAdapter {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &GeminiAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (g *GeminiAdapter) Name() string        { return "Gemini" }
func (g *GeminiAdapter) Type() ProviderType { return ProviderGemini }

func (g *GeminiAdapter) Models(ctx context.Context) ([]protocol.Model, error) {
	return []protocol.Model{
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro"},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash"},
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash"},
	}, nil
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
}

func (g *GeminiAdapter) Complete(ctx context.Context, req protocol.ChatRequest, emit func(StreamDelta) error) error {
	var contents []geminiContent
	var system *geminiContent

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			system = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
		} else {
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: msg.Content}},
			})
		}
	}

	modelName := strings.TrimPrefix(req.Model, "gemini/")
	modelName = strings.TrimPrefix(modelName, "google/")
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	bodyData, err := json.Marshal(geminiRequest{
		Contents:          contents,
		SystemInstruction: system,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", g.baseURL, modelName, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gemini error (%d): %s", resp.StatusCode, string(out))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")

		var response struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(raw), &response); err == nil {
			for _, candidate := range response.Candidates {
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						_ = emit(StreamDelta{
							Content: part.Text,
						})
					}
				}
			}
		}
	}
	return scanner.Err()
}
