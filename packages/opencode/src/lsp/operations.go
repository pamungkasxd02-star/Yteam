package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}
type Symbol struct {
	Name     string   `json:"name"`
	Kind     int      `json:"kind"`
	Location Location `json:"location"`
}

func FileURI(path string) string { return (&url.URL{Scheme: "file", Path: path}).String() }

func (c *Client) Notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	header := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"
	if _, err := io.WriteString(c.in, header); err != nil {
		return err
	}
	_, err = c.in.Write(body)
	return err
}

func (c *Client) DidOpen(path, language, text string) error {
	return c.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": FileURI(path), "languageId": language, "version": 1, "text": text}})
}
func (c *Client) Definition(ctx context.Context, path string, position Position) ([]Location, error) {
	var result []Location
	err := c.Request(ctx, "textDocument/definition", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}, "position": position}, &result)
	return result, err
}
func (c *Client) References(ctx context.Context, path string, position Position) ([]Location, error) {
	var result []Location
	err := c.Request(ctx, "textDocument/references", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}, "position": position, "context": map[string]bool{"includeDeclaration": true}}, &result)
	return result, err
}
func (c *Client) Hover(ctx context.Context, path string, position Position) (any, error) {
	var result any
	err := c.Request(ctx, "textDocument/hover", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}, "position": position}, &result)
	return result, err
}
func (c *Client) DocumentSymbol(ctx context.Context, path string) ([]Symbol, error) {
	var result []Symbol
	err := c.Request(ctx, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}}, &result)
	return result, err
}
func (c *Client) WorkspaceSymbol(ctx context.Context, query string) ([]Symbol, error) {
	var result []Symbol
	err := c.Request(ctx, "workspace/symbol", map[string]string{"query": query}, &result)
	return result, err
}

type OperationInput struct {
	Operation string `json:"operation"`
	FilePath  string `json:"filePath"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Query     string `json:"query,omitempty"`
}

var supportedOperations = map[string]bool{"goToDefinition": true, "findReferences": true, "hover": true, "documentSymbol": true, "workspaceSymbol": true}

func (m *Manager) Execute(ctx context.Context, input OperationInput, root string) (any, error) {
	if !supportedOperations[input.Operation] {
		return nil, errors.New("unsupported LSP operation")
	}
	if input.Operation != "workspaceSymbol" && (input.Line < 1 || input.Character < 1) {
		return nil, errors.New("line and character are 1-based and required")
	}
	path := input.FilePath
	if input.Operation != "workspaceSymbol" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
	}
	m.mu.RLock()
	var client *Client
	for _, candidate := range m.clients {
		client = candidate
		break
	}
	m.mu.RUnlock()
	if client == nil {
		return nil, errors.New("no connected LSP server")
	}
	position := Position{Line: input.Line - 1, Character: input.Character - 1}
	switch input.Operation {
	case "goToDefinition":
		return client.Definition(ctx, path, position)
	case "findReferences":
		return client.References(ctx, path, position)
	case "hover":
		return client.Hover(ctx, path, position)
	case "documentSymbol":
		return client.DocumentSymbol(ctx, path)
	case "workspaceSymbol":
		return client.WorkspaceSymbol(ctx, input.Query)
	}
	return nil, errors.New("unsupported LSP operation")
}
