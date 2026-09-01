package lsp

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadContentLength(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("Content-Type: application/vscode-jsonrpc\r\nContent-Length: 17\r\n\r\n"))
	length, err := readContentLength(reader)
	if err != nil || length != 17 {
		t.Fatalf("length = %d, err = %v", length, err)
	}
}
