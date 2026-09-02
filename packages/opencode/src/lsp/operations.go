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
	"strings"
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

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

type DiagnosticReport struct {
	Kind  string       `json:"kind,omitempty"`
	Items []Diagnostic `json:"items"`
}

type CodeAction struct {
	Title string `json:"title"`
	Kind  string `json:"kind,omitempty"`
	Data  any    `json:"data,omitempty"`
}

type CallHierarchyItem struct {
	Name           string `json:"name"`
	Kind           int    `json:"kind"`
	URI            string `json:"uri"`
	Range          Range  `json:"range"`
	SelectionRange Range  `json:"selectionRange"`
	Detail         string `json:"detail,omitempty"`
}

type CallHierarchyIncomingCall struct {
	FromRanges []Range           `json:"fromRanges"`
	From       CallHierarchyItem `json:"from"`
}

type CallHierarchyOutgoingCall struct {
	ToRanges []Range           `json:"fromRanges"`
	To       CallHierarchyItem `json:"to"`
}

func FileURI(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

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

func (c *Client) Diagnostics(ctx context.Context, path string) (DiagnosticReport, error) {
	var result DiagnosticReport
	err := c.Request(ctx, "textDocument/diagnostic", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}}, &result)
	return result, err
}

func (c *Client) Implementation(ctx context.Context, path string, position Position) ([]Location, error) {
	var result []Location
	err := c.Request(ctx, "textDocument/implementation", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}, "position": position}, &result)
	return result, err
}

func (c *Client) CodeActions(ctx context.Context, path string, rng Range) ([]CodeAction, error) {
	var result []CodeAction
	err := c.Request(ctx, "textDocument/codeAction", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}, "range": rng, "context": map[string]any{"diagnostics": []Diagnostic{}}}, &result)
	return result, err
}

func (c *Client) PrepareCallHierarchy(ctx context.Context, path string, position Position) ([]CallHierarchyItem, error) {
	var result []CallHierarchyItem
	err := c.Request(ctx, "textDocument/prepareCallHierarchy", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}, "position": position}, &result)
	return result, err
}

func (c *Client) IncomingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyIncomingCall, error) {
	var result []CallHierarchyIncomingCall
	err := c.Request(ctx, "callHierarchy/incomingCalls", map[string]any{"item": item}, &result)
	return result, err
}

func (c *Client) OutgoingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyOutgoingCall, error) {
	var result []CallHierarchyOutgoingCall
	err := c.Request(ctx, "callHierarchy/outgoingCalls", map[string]any{"item": item}, &result)
	return result, err
}

type OperationInput struct {
	Operation    string             `json:"operation"`
	FilePath     string             `json:"filePath"`
	Line         int                `json:"line"`
	Character    int                `json:"character"`
	Query        string             `json:"query,omitempty"`
	EndLine      int                `json:"endLine,omitempty"`
	EndCharacter int                `json:"endCharacter,omitempty"`
	Item         *CallHierarchyItem `json:"item,omitempty"`
}

var supportedOperations = map[string]bool{
	"goToDefinition": true, "findReferences": true, "hover": true,
	"documentSymbol": true, "workspaceSymbol": true, "implementation": true,
	"codeAction": true, "prepareCallHierarchy": true, "incomingCalls": true,
	"outgoingCalls": true,
	"diagnostics":   true,
}

func (m *Manager) Execute(ctx context.Context, input OperationInput, root string) (any, error) {
	if !supportedOperations[input.Operation] {
		return nil, errors.New("unsupported LSP operation")
	}
	if (input.Operation == "incomingCalls" || input.Operation == "outgoingCalls") && input.Item == nil {
		return nil, errors.New("call hierarchy item is required")
	}
	needsPosition := map[string]bool{"goToDefinition": true, "findReferences": true, "hover": true, "implementation": true, "codeAction": true, "prepareCallHierarchy": true}
	if needsPosition[input.Operation] && (input.Line < 1 || input.Character < 1) {
		return nil, errors.New("line and character are 1-based and required")
	}
	path := input.FilePath
	needsFile := input.Operation != "workspaceSymbol" && input.Operation != "incomingCalls" && input.Operation != "outgoingCalls"
	if needsFile {
		if strings.TrimSpace(path) == "" {
			return nil, errors.New("file path is required")
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(absoluteRoot, absolutePath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, errors.New("LSP file path is outside project root")
		}
		path = absolutePath
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
	}
	client := m.selectClient(path, root)
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
	case "diagnostics":
		return client.Diagnostics(ctx, path)
	case "implementation":
		return client.Implementation(ctx, path, position)
	case "codeAction":
		end := Range{Start: position, End: position}
		if input.EndLine > 0 {
			end.End.Line = input.EndLine - 1
		}
		if input.EndCharacter > 0 {
			end.End.Character = input.EndCharacter - 1
		}
		if end.End.Line < end.Start.Line || (end.End.Line == end.Start.Line && end.End.Character < end.Start.Character) {
			return nil, errors.New("code action end position precedes start position")
		}
		return client.CodeActions(ctx, path, end)
	case "prepareCallHierarchy":
		return client.PrepareCallHierarchy(ctx, path, position)
	case "incomingCalls":
		return client.IncomingCalls(ctx, *input.Item)
	case "outgoingCalls":
		return client.OutgoingCalls(ctx, *input.Item)
	}
	return nil, errors.New("unsupported LSP operation")
}
