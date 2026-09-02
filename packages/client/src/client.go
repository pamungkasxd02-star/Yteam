package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/snapshot"
	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), token: token, httpClient: http.DefaultClient}
}

func NewWithHTTPClient(baseURL, token string, httpClient *http.Client) *Client {
	client := New(baseURL, token)
	if httpClient != nil {
		client.httpClient = httpClient
	}
	return client
}

func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/health", nil, &result)
	return result, err
}

func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/status", nil, &result)
	return result, err
}

func (c *Client) Sessions(ctx context.Context) ([]session.Session, error) {
	var result []session.Session
	err := c.doJSON(ctx, http.MethodGet, "/api/session", nil, &result)
	return result, err
}

func (c *Client) Session(ctx context.Context, id string) (*session.Session, error) {
	var result session.Session
	err := c.doJSON(ctx, http.MethodGet, sessionPath(id), nil, &result)
	return &result, err
}

func (c *Client) Messages(ctx context.Context, id string) ([]session.Message, error) {
	var result []session.Message
	err := c.doJSON(ctx, http.MethodGet, sessionPath(id)+"/message", nil, &result)
	return result, err
}

func (c *Client) Models(ctx context.Context) ([]protocol.Model, error) {
	var result []protocol.Model
	err := c.doJSON(ctx, http.MethodGet, "/api/models", nil, &result)
	return result, err
}

func (c *Client) Tools(ctx context.Context) ([]schema.ToolDefinition, error) {
	var result []schema.ToolDefinition
	err := c.doJSON(ctx, http.MethodGet, "/api/tools", nil, &result)
	return result, err
}

func (c *Client) Agents(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/agent", nil, &result)
	return result, err
}

func (c *Client) SetModel(ctx context.Context, model string) (string, error) {
	var result struct {
		Current string `json:"current"`
	}
	err := c.doJSON(ctx, http.MethodPost, "/api/model", map[string]string{"model": model}, &result)
	return result.Current, err
}

func (c *Client) SetAgent(ctx context.Context, agent string) (string, error) {
	var result struct {
		Current string `json:"current"`
	}
	err := c.doJSON(ctx, http.MethodPost, "/api/agent", map[string]string{"name": agent}, &result)
	return result.Current, err
}

func (c *Client) NewSession(ctx context.Context) (*session.Session, error) {
	var result session.Session
	err := c.doJSON(ctx, http.MethodPost, "/api/session", nil, &result)
	return &result, err
}

func (c *Client) RenameSession(ctx context.Context, id, title string) (*session.Session, error) {
	var result session.Session
	err := c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/rename", map[string]string{"title": title}, &result)
	return &result, err
}

func (c *Client) ForkSession(ctx context.Context, id string) (*session.Session, error) {
	var result session.Session
	err := c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/fork", nil, &result)
	return &result, err
}

func (c *Client) DeleteSession(ctx context.Context, id string) error {
	response, err := c.do(ctx, http.MethodDelete, sessionPath(id), nil, nil)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func (c *Client) Prompt(ctx context.Context, id, content string, delivery session.Delivery) (string, error) {
	var result strings.Builder
	path := sessionPath(id) + "/prompt"
	if delivery == "" {
		delivery = session.DeliveryQueue
	}
	response, err := c.do(ctx, http.MethodPost, path, map[string]any{"content": content, "delivery": delivery}, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.Copy(&result, io.LimitReader(response.Body, 16*1024*1024))
	if err != nil {
		return "", err
	}
	_ = data
	return result.String(), nil
}

func (c *Client) Input(ctx context.Context, id, content string, delivery session.Delivery) (session.Input, error) {
	var result session.Input
	err := c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/input", map[string]any{"content": content, "delivery": delivery}, &result)
	return result, err
}

func (c *Client) PendingInputs(ctx context.Context, id string) ([]session.Input, error) {
	var result []session.Input
	err := c.doJSON(ctx, http.MethodGet, sessionPath(id)+"/input", nil, &result)
	return result, err
}

func (c *Client) PromoteInputs(ctx context.Context, id string) ([]session.Input, error) {
	var result []session.Input
	err := c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/input/promote", nil, &result)
	return result, err
}

func (c *Client) Interrupt(ctx context.Context, id string) error {
	response, err := c.do(ctx, http.MethodPost, sessionPath(id)+"/interrupt", nil, nil)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func (c *Client) PendingPermissions(ctx context.Context, id string) ([]permission.Request, error) {
	var result []permission.Request
	err := c.doJSON(ctx, http.MethodGet, sessionPath(id)+"/permission", nil, &result)
	return result, err
}

func (c *Client) ReplyPermission(ctx context.Context, id, requestID string, reply permission.Reply) error {
	return c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/permission/"+escape(requestID), map[string]any{"reply": reply}, &struct{}{})
}

func (c *Client) PendingQuestions(ctx context.Context, id string) ([]schema.QuestionRequest, error) {
	var result []schema.QuestionRequest
	err := c.doJSON(ctx, http.MethodGet, sessionPath(id)+"/question", nil, &result)
	return result, err
}

func (c *Client) ReplyQuestion(ctx context.Context, id, requestID string, answers []schema.QuestionAnswer) error {
	return c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/question/"+escape(requestID)+"/reply", schema.QuestionReply{Answers: answers}, &struct{}{})
}

func (c *Client) RejectQuestion(ctx context.Context, id, requestID string) error {
	response, err := c.do(ctx, http.MethodPost, sessionPath(id)+"/question/"+escape(requestID)+"/reject", nil, nil)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func (c *Client) Compact(ctx context.Context, id, summary string, keep int) (*session.Compaction, error) {
	var result session.Compaction
	err := c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/compact", map[string]any{"summary": summary, "keep": keep}, &result)
	return &result, err
}

func (c *Client) Export(ctx context.Context, id, format string) ([]byte, error) {
	path := sessionPath(id) + "/export?format=" + url.QueryEscape(format)
	response, err := c.do(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(io.LimitReader(response.Body, 16*1024*1024))
}

func (c *Client) Usage(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/provider/usage", nil, &result)
	return result, err
}

func (c *Client) GitStatus(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/git", nil, &result)
	return result, err
}

func (c *Client) GitDiff(ctx context.Context) (string, error) {
	return c.text(ctx, http.MethodGet, "/api/git/diff", nil)
}

func (c *Client) GitLog(ctx context.Context, count int) (string, error) {
	return c.text(ctx, http.MethodGet, "/api/git/log?count="+strconv.Itoa(count), nil)
}

func (c *Client) Skills(ctx context.Context) ([]map[string]any, error) {
	var result []map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/skills", nil, &result)
	return result, err
}

func (c *Client) MCP(ctx context.Context) ([]map[string]any, error) {
	var result []map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/mcp", nil, &result)
	return result, err
}

func (c *Client) LSP(ctx context.Context) ([]map[string]any, error) {
	var result []map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/lsp", nil, &result)
	return result, err
}

func (c *Client) Plugins(ctx context.Context) ([]map[string]any, error) {
	var result []map[string]any
	err := c.doJSON(ctx, http.MethodGet, "/api/plugin", nil, &result)
	return result, err
}

func (c *Client) ExecuteLSP(ctx context.Context, input map[string]any) (any, error) {
	var result any
	err := c.doJSON(ctx, http.MethodPost, "/api/lsp", input, &result)
	return result, err
}

func (c *Client) Snapshot(ctx context.Context, id string) (snapshot.Manifest, error) {
	var result snapshot.Manifest
	err := c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/snapshot", nil, &result)
	return result, err
}

func (c *Client) SnapshotDiff(ctx context.Context, id, snapshotID string) (string, error) {
	return c.text(ctx, http.MethodGet, sessionPath(id)+"/snapshot/"+escape(snapshotID), nil)
}

func (c *Client) Revert(ctx context.Context, id string) (*session.RevertState, error) {
	var result *session.RevertState
	err := c.doJSON(ctx, http.MethodGet, sessionPath(id)+"/revert", nil, &result)
	return result, err
}

func (c *Client) StageRevert(ctx context.Context, id, messageID, diff string) (*session.RevertState, error) {
	var result session.RevertState
	err := c.doJSON(ctx, http.MethodPost, sessionPath(id)+"/revert/stage", map[string]string{"message_id": messageID, "diff": diff}, &result)
	return &result, err
}

func (c *Client) ClearRevert(ctx context.Context, id string) error {
	return c.sessionAction(ctx, sessionPath(id)+"/revert/clear")
}

func (c *Client) CommitRevert(ctx context.Context, id string) error {
	return c.sessionAction(ctx, sessionPath(id)+"/revert/commit")
}

type EventStream struct {
	response *http.Response
	scanner  *bufio.Scanner
	closed   bool
}

func (c *Client) Events(ctx context.Context, id string, after uint64) (*EventStream, error) {
	path := "/api/event"
	if id != "" {
		path = sessionPath(id) + "/event?after=" + strconv.FormatUint(after, 10)
	}
	response, err := c.do(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	return &EventStream{response: response, scanner: scanner}, nil
}

func (s *EventStream) Next() (schema.Event, error) {
	if s.closed {
		return schema.Event{}, io.EOF
	}
	var eventType string
	var data strings.Builder
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if line == "" {
			if data.Len() == 0 {
				continue
			}
			var result schema.Event
			if err := json.Unmarshal([]byte(data.String()), &result); err != nil {
				return schema.Event{}, err
			}
			if result.Type == "" {
				result.Type = eventType
			}
			return result, nil
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := s.scanner.Err(); err != nil {
		return schema.Event{}, err
	}
	return schema.Event{}, io.EOF
}

func (s *EventStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.response.Body.Close()
}

func (c *Client) doJSON(ctx context.Context, method, path string, input, output any) error {
	response, err := c.do(ctx, method, path, input, output)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 16*1024*1024)).Decode(output)
}

func (c *Client) text(ctx context.Context, method, path string, input any) (string, error) {
	response, err := c.do(ctx, method, path, input, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16*1024*1024))
	return string(data), err
}

func (c *Client) sessionAction(ctx context.Context, path string) error {
	response, err := c.do(ctx, http.MethodPost, path, nil, nil)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func (c *Client) do(ctx context.Context, method, path string, input, output any) (*http.Response, error) {
	if c == nil || strings.TrimSpace(c.baseURL) == "" {
		return nil, errors.New("client base URL is empty")
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 400 {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = response.Status
		}
		return nil, fmt.Errorf("server %s: %s", response.Status, message)
	}
	_ = output
	return response, nil
}

func sessionPath(id string) string { return "/api/session/" + escape(id) }
func escape(value string) string   { return url.PathEscape(strings.TrimSpace(value)) }
