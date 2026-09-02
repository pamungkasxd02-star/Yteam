package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Config struct {
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment,omitempty"`
	WorkingDir  string            `json:"cwd,omitempty"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type Client struct {
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	mu   sync.Mutex
	next int64
}

func Start(ctx context.Context, cfg Config) (*Client, error) {
	if len(cfg.Command) == 0 || strings.TrimSpace(cfg.Command[0]) == "" {
		return nil, errors.New("plugin command is empty")
	}
	cmd := exec.CommandContext(ctx, cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir, cmd.Stderr = cfg.WorkingDir, io.Discard
	cmd.Env = append([]string(nil), os.Environ()...)
	for key, value := range cfg.Environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Client{cmd: cmd, in: in, out: bufio.NewReader(stdout)}, nil
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	data, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	if err != nil {
		return err
	}
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		return err
	}
	value, errorsCh := make(chan []byte, 1), make(chan error, 1)
	go func() {
		line, err := c.out.ReadBytes('\n')
		if err != nil {
			errorsCh <- err
		} else {
			value <- line
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errorsCh:
		return err
	case line := <-value:
		var response struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("plugin %d: %s", response.Error.Code, response.Error.Message)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(response.Result, result)
	}
}

func (c *Client) Initialize(ctx context.Context, name string) error {
	var result map[string]any
	return c.Call(ctx, "initialize", map[string]any{"name": name, "version": "0.1.0", "capabilities": map[string]any{}}, &result)
}
func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	var result struct {
		Tools []Tool `json:"tools"`
	}
	err := c.Call(ctx, "tools/list", map[string]any{}, &result)
	return result.Tools, err
}
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	var result struct {
		Content []struct {
			Text string `json:"text,omitempty"`
			Data string `json:"data,omitempty"`
		} `json:"content"`
		Result  string `json:"result,omitempty"`
		IsError bool   `json:"isError,omitempty"`
	}
	if err := c.Call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result); err != nil {
		return "", err
	}
	items := []string{}
	for _, item := range result.Content {
		if item.Text != "" {
			items = append(items, item.Text)
		}
		if item.Data != "" {
			items = append(items, item.Data)
		}
	}
	if result.Result != "" {
		items = append(items, result.Result)
	}
	output := strings.Join(items, "\n")
	if result.IsError {
		return output, errors.New(output)
	}
	return output, nil
}
func (c *Client) Close() error {
	if c.in != nil {
		_ = c.in.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
