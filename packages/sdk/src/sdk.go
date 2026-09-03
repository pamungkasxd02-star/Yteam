package sdk

import (
	"context"

	"github.com/pamungkasxd02-star/Yteam/packages/client/src"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/protocol/src"
)

// Client is a high-level programmatic SDK client for embedding Yteam.
type Client struct {
	raw *client.Client
}

func New(serverURL, token string) *Client {
	return &Client{
		raw: client.New(serverURL, token),
	}
}

func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	return c.raw.Health(ctx)
}

func (c *Client) CreateSession(ctx context.Context) (*session.Session, error) {
	return c.raw.NewSession(ctx)
}

func (c *Client) ListSessions(ctx context.Context) ([]session.Session, error) {
	return c.raw.Sessions(ctx)
}

func (c *Client) Prompt(ctx context.Context, sessionID, text string) (string, error) {
	return c.raw.Prompt(ctx, sessionID, text, session.DeliveryQueue)
}

func (c *Client) Models(ctx context.Context) ([]protocol.Model, error) {
	return c.raw.Models(ctx)
}

func (c *Client) FreeModels() []protocol.Model {
	return []protocol.Model{
		{ID: "mimo-v2.5-free", Name: "Mimo v2.5 Free (OpenCode Native)", Description: "OpenCode default free model"},
		{ID: "gemini-2.5-flash", Name: "Google Gemini 2.5 Flash Free", Description: "Fast Google Gemini free tier"},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku (Fast)", Description: "Anthropic fast reasoning model"},
		{ID: "llama-3.3-70b-free", Name: "Llama 3.3 70B Free", Description: "High-capability open model free tier"},
		{ID: "qwen-2.5-coder-32b-free", Name: "Qwen 2.5 Coder 32B Free", Description: "Specialized coding LLM free tier"},
		{ID: "deepseek-v3-free", Name: "DeepSeek V3 Free", Description: "OpenCode Zen free provider"},
	}
}

func (c *Client) Status(ctx context.Context) (client.Status, error) {
	return c.raw.Status(ctx)
}
