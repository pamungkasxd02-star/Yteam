package tui

import (
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
	if data[0] == 27 {
		if len(data) < 2 {
			if err := r.readMore(2); err != nil {
				return Key{}, err
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
		if len(data) < 3 {
			if err := r.readMore(3); err != nil {
				return Key{}, err
			}
			data = r.pending
		}
		if len(data) >= 3 {
			switch data[2] {
			case 'A':
				r.pending = data[3:]
				return Key{Kind: KeyUp}, nil
			case 'B':
				r.pending = data[3:]
				return Key{Kind: KeyDown}, nil
			case 'C':
				r.pending = data[3:]
				return Key{Kind: KeyRight}, nil
			case 'D':
				r.pending = data[3:]
				return Key{Kind: KeyLeft}, nil
			case 'H':
				r.pending = data[3:]
				return Key{Kind: KeyHome}, nil
			case 'F':
				r.pending = data[3:]
				return Key{Kind: KeyEnd}, nil
			}
		}
		if data[2] == '3' {
			if len(data) < 4 {
				if err := r.readMore(4); err != nil {
					return Key{}, err
				}
				data = r.pending
			}
		}
		if len(data) >= 4 && data[2] == '3' && data[3] == '~' {
			r.pending = data[4:]
			return Key{Kind: KeyDelete}, nil
		}
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
