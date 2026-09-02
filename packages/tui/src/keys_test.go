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

func TestKeyReaderDecodesPageMovementAndBareEscape(t *testing.T) {
	reader := NewKeyReader(strings.NewReader("\x1b[5~\x1b[6~\x1b"))
	key, err := reader.ReadKey()
	if err != nil || key.Kind != KeyPageUp {
		t.Fatalf("page up = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyPageDown {
		t.Fatalf("page down = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyEscape {
		t.Fatalf("escape = %#v, %v", key, err)
	}
}

func TestKeyReaderDecodesBracketedPaste(t *testing.T) {
	reader := NewKeyReader(strings.NewReader("\x1b[200~hello\n世界\x1b[201~x"))
	key, err := reader.ReadKey()
	if err != nil || key.Kind != KeyPaste || key.Text != "hello\n世界" {
		t.Fatalf("paste = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyText || key.Text != "x" {
		t.Fatalf("after paste = %#v, %v", key, err)
	}
}

func TestKeyReaderDecodesWordKeys(t *testing.T) {
	reader := NewKeyReader(strings.NewReader("\x1b[1;5D\x1b[1;3C\x17\x1b[3;5~"))
	for index, expected := range []KeyKind{KeyWordLeft, KeyWordRight, KeyDeleteWordBackward, KeyDeleteWordForward} {
		key, err := reader.ReadKey()
		if err != nil || key.Kind != expected {
			t.Fatalf("key %d = %#v, %v; want %d", index, key, err, expected)
		}
	}
}

func TestKeyReaderDecodesCtrlQ(t *testing.T) {
	key, err := NewKeyReader(strings.NewReader("\x11")).ReadKey()
	if err != nil || key.Kind != KeyCtrlQ {
		t.Fatalf("ctrl-q = %#v, %v", key, err)
	}
}

func TestKeyReaderDropsOversizedPasteAndKeepsFollowingKey(t *testing.T) {
	reader := NewKeyReader(strings.NewReader("\x1b[200~123456789\x1b[201~x"))
	reader.maxPasteBytes = 8
	key, err := reader.ReadKey()
	if err != nil || key.Kind != KeyPaste || key.Text != "" {
		t.Fatalf("oversized paste = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyText || key.Text != "x" {
		t.Fatalf("following key = %#v, %v", key, err)
	}
}

func TestKeyReaderRecognizesSplitPasteEndMarkerAfterDiscard(t *testing.T) {
	reader := NewKeyReader(&chunkReader{chunks: [][]byte{
		[]byte("\x1b[200~123456789\x1b[20"),
		[]byte("1~x"),
	}})
	reader.maxPasteBytes = 8
	key, err := reader.ReadKey()
	if err != nil || key.Kind != KeyPaste || key.Text != "" {
		t.Fatalf("oversized split paste = %#v, %v", key, err)
	}
	key, err = reader.ReadKey()
	if err != nil || key.Kind != KeyText || key.Text != "x" {
		t.Fatalf("key after split marker = %#v, %v", key, err)
	}
}

func TestKeyReaderResetClearsPendingPaste(t *testing.T) {
	reader := NewKeyReader(strings.NewReader("\x1b[200~unfinished"))
	if _, err := reader.ReadKey(); err != nil {
		t.Fatal(err)
	}
	reader.Reset()
	reader.reader = strings.NewReader("x")
	key, err := reader.ReadKey()
	if err != nil || key.Kind != KeyText || key.Text != "x" {
		t.Fatalf("reset key = %#v, %v", key, err)
	}
}
