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

// OllamaAdapter handles local/remote Ollama streaming API.
type OllamaAdapter struct {
	baseURL    string
	httpClient *http.Client
}

func NewOllamaAdapter(baseURL string) *OllamaAdapter {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaAdapter{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (o *OllamaAdapter) Name() string        { return "Ollama" }
func (o *OllamaAdapter) Type() ProviderType { return ProviderOllama }

func (o *OllamaAdapter) Models(ctx context.Context) ([]protocol.Model, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return []protocol.Model{
			{ID: "ollama/llama3", Name: "Llama 3"},
			{ID: "ollama/qwen2.5-coder", Name: "Qwen 2.5 Coder"},
		}, nil
	}
	defer resp.Body.Close()

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, err
	}
	var res []protocol.Model
	for _, m := range tagsResp.Models {
		res = append(res, protocol.Model{
			ID:   "ollama/" + m.Name,
			Name: m.Name,
		})
	}
	return res, nil
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

func (o *OllamaAdapter) Complete(ctx context.Context, req protocol.ChatRequest, emit func(StreamDelta) error) error {
	var messages []ollamaMessage
	for _, msg := range req.Messages {
		messages = append(messages, ollamaMessage{Role: string(msg.Role), Content: msg.Content})
	}
	modelName := strings.TrimPrefix(req.Model, "ollama/")
	if modelName == "" {
		modelName = "llama3"
	}

	bodyData, err := json.Marshal(ollamaRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/chat", bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama error (%d): %s", resp.StatusCode, string(out))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := json.Unmarshal([]byte(line), &event); err == nil {
			if event.Message.Content != "" {
				_ = emit(StreamDelta{
					Content: event.Message.Content,
				})
			}
			if event.Done {
				break
			}
		}
	}
	return scanner.Err()
}
