package tui

import (
	"io"
	"strings"
	"testing"
)

func TestKeyReaderDecodesUTF8AndANSI(t *testing.T) {
	reader := NewKeyReader(strings.NewReader("é\x1b[A\x1b[3~\x03"))
	key, err := reader.ReadKey()
	if err != nil || key.Kind != KeyText || key.Text != "é" {
		t.Fatalf("utf8 = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyUp {
		t.Fatalf("up = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyDelete {
		t.Fatalf("delete = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyCtrlC {
		t.Fatalf("ctrl-c = %#v, %v", key, err)
	}
}

type chunkReader struct {
	chunks [][]byte
	index  int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.index]
	r.index++
	copy(p, chunk)
	return len(chunk), nil
}

func TestKeyReaderHandlesFragmentedEscapeSequence(t *testing.T) {
	reader := NewKeyReader(&chunkReader{chunks: [][]byte{{27}, {'['}, {'A'}}})
	key, err := reader.ReadKey()
	if err != nil || key.Kind != KeyUp {
		t.Fatalf("key = %#v, err = %v", key, err)
	}
}

func TestKeyReaderMapsLineFeedToNewline(t *testing.T) {
	key, err := NewKeyReader(strings.NewReader("\n")).ReadKey()
	if err != nil || key.Kind != KeyCtrlJ {
		t.Fatalf("key = %#v, err = %v", key, err)
	}
}
