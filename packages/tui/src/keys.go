package tui

import (
	"bytes"
	"io"
	"unicode/utf8"
)

type KeyKind int

const (
	KeyText KeyKind = iota
	KeyEnter
	KeyBackspace
	KeyDelete
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyTab
	KeyEscape
	KeyCtrlC
	KeyCtrlJ
	KeyCtrlP
	KeyPageUp
	KeyPageDown
	KeyPaste
	KeyWordLeft
	KeyWordRight
	KeyDeleteWordBackward
	KeyDeleteWordForward
)

type Key struct {
	Kind KeyKind
	Text string
}

type KeyReader struct {
	reader  io.Reader
	pending []byte
}

func NewKeyReader(reader io.Reader) *KeyReader { return &KeyReader{reader: reader} }

func (r *KeyReader) ReadKey() (Key, error) {
	if err := r.readMore(1); err != nil && len(r.pending) == 0 {
		return Key{}, err
	}
	data := r.pending
	if data[0] == 3 {
		r.pending = data[1:]
		return Key{Kind: KeyCtrlC}, nil
	}
	if data[0] == 13 {
		r.pending = data[1:]
		return Key{Kind: KeyEnter}, nil
	}
	if data[0] == 8 || data[0] == 127 {
		r.pending = data[1:]
		return Key{Kind: KeyBackspace}, nil
	}
	if data[0] == 9 {
		r.pending = data[1:]
		return Key{Kind: KeyTab}, nil
	}
	if data[0] == 10 {
		r.pending = data[1:]
		return Key{Kind: KeyCtrlJ}, nil
	}
	if data[0] == 16 {
		r.pending = data[1:]
		return Key{Kind: KeyCtrlP}, nil
	}
	if data[0] == 23 {
		r.pending = data[1:]
		return Key{Kind: KeyDeleteWordBackward}, nil
	}
	if data[0] == 27 {
		if len(data) < 2 {
			if err := r.readMore(2); err != nil {
				// EOF after a lone escape is a valid bare Escape key.
				r.pending = data[1:]
				return Key{Kind: KeyEscape}, nil
			}
			data = r.pending
		}
		return r.escape(data)
	}
	if data[0] >= 0xC0 && data[0] <= 0xF4 {
		width := utf8.RuneLen(rune(data[0]))
		if width > 1 && len(data) < width {
			if err := r.readMore(width); err != nil {
				return Key{}, err
			}
			data = r.pending
		}
	}
	runeValue, size := utf8.DecodeRune(data)
	if runeValue == utf8.RuneError && size == 1 {
		r.pending = data[1:]
		return Key{Kind: KeyText, Text: string(data[:1])}, nil
	}
	r.pending = data[size:]
	return Key{Kind: KeyText, Text: string(runeValue)}, nil
}

func (r *KeyReader) escape(data []byte) (Key, error) {
	if len(data) == 1 {
		r.pending = data[1:]
		return Key{Kind: KeyEscape}, nil
	}
	if data[1] == '[' {
		return r.csi(data)
	}
	if data[1] == 'O' && len(data) >= 3 {
		if data[2] == 'H' {
			r.pending = data[3:]
			return Key{Kind: KeyHome}, nil
		}
		if data[2] == 'F' {
			r.pending = data[3:]
			return Key{Kind: KeyEnd}, nil
		}
	}
	r.pending = data[1:]
	return Key{Kind: KeyEscape}, nil
}

func (r *KeyReader) csi(data []byte) (Key, error) {
	for {
		if bytes.HasPrefix(data, []byte("\x1b[200~")) {
			endMarker := []byte("\x1b[201~")
			if end := bytes.Index(data[6:], endMarker); end >= 0 {
				start := 6
				end += start
				text := string(data[start:end])
				r.pending = data[end+len(endMarker):]
				return Key{Kind: KeyPaste, Text: text}, nil
			}
			if err := r.readMore(len(data) + 1); err != nil {
				text := data[6:]
				// A truncated bracketed paste is still user input. Returning it
				// is preferable to dropping the paste at EOF.
				r.pending = nil
				return Key{Kind: KeyPaste, Text: string(text)}, nil
			}
			data = r.pending
			continue
		}
		for index := 2; index < len(data); index++ {
			if data[index] < 0x40 || data[index] > 0x7e {
				continue
			}
			sequence := data[2 : index+1]
			r.pending = data[index+1:]
			switch string(sequence) {
			case "A":
				return Key{Kind: KeyUp}, nil
			case "B":
				return Key{Kind: KeyDown}, nil
			case "C":
				return Key{Kind: KeyRight}, nil
			case "D":
				return Key{Kind: KeyLeft}, nil
			case "H", "1~", "7~":
				return Key{Kind: KeyHome}, nil
			case "F", "4~", "8~":
				return Key{Kind: KeyEnd}, nil
			case "3~":
				return Key{Kind: KeyDelete}, nil
			case "5~":
				return Key{Kind: KeyPageUp}, nil
			case "6~":
				return Key{Kind: KeyPageDown}, nil
			case "1;3D", "1;5D", "5D":
				return Key{Kind: KeyWordLeft}, nil
			case "1;3C", "1;5C", "5C":
				return Key{Kind: KeyWordRight}, nil
			case "3;3~", "3;5~":
				return Key{Kind: KeyDeleteWordForward}, nil
			}
			return Key{Kind: KeyEscape}, nil
		}
		if err := r.readMore(len(data) + 1); err != nil {
			r.pending = data[1:]
			return Key{Kind: KeyEscape}, nil
		}
		data = r.pending
	}
}

func (r *KeyReader) readMore(minimum int) error {
	for len(r.pending) < minimum {
		buffer := make([]byte, 64)
		n, err := r.reader.Read(buffer)
		if n > 0 {
			r.pending = append(r.pending, buffer[:n]...)
		}
		if err != nil {
			if len(r.pending) >= minimum {
				return nil
			}
			return err
		}
		if n == 0 {
			continue
		}
	}
	return nil
}
