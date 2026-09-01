package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type Client struct {
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	mu   sync.Mutex
	next int64
}

func Start(ctx context.Context, command []string, directory string) (*Client, error) {
	if len(command) == 0 {
		return nil, errors.New("LSP command is empty")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = directory
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Client{cmd: cmd, in: in, out: bufio.NewReader(stdout)}, nil
}
func (c *Client) Request(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	id := c.next
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	header := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"
	if _, err := io.WriteString(c.in, header); err != nil {
		return err
	}
	if _, err := c.in.Write(body); err != nil {
		return err
	}
	type response struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	value := make(chan response, 1)
	errorsCh := make(chan error, 1)
	go func() {
		length, err := readContentLength(c.out)
		if err != nil {
			errorsCh <- err
			return
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(c.out, data); err != nil {
			errorsCh <- err
			return
		}
		var reply response
		if err := json.Unmarshal(data, &reply); err != nil {
			errorsCh <- err
			return
		}
		value <- reply
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errorsCh:
		return err
	case reply := <-value:
		if reply.Error != nil {
			return fmt.Errorf("LSP %d: %s", reply.Error.Code, reply.Error.Message)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(reply.Result, result)
	}
}

func readContentLength(reader *bufio.Reader) (int, error) {
	length := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if length < 0 {
				return 0, errors.New("LSP response has no Content-Length")
			}
			return length, nil
		}
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return 0, err
			}
		}
	}
}
func (c *Client) Initialize(ctx context.Context, rootURI string) error {
	var result map[string]any
	return c.Request(ctx, "initialize", map[string]any{"processId": nil, "rootUri": rootURI, "capabilities": map[string]any{}}, &result)
}
func (c *Client) Shutdown(ctx context.Context) error { return c.Request(ctx, "shutdown", nil, nil) }
func (c *Client) Close() error {
	if c.in != nil {
		_ = c.in.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}
