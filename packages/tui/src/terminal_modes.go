package tui

import (
	"fmt"
	"io"
	"os"
	"sync"
)

const (
	bracketedPasteEnable  = "\x1b[?2004h"
	bracketedPasteDisable = "\x1b[?2004l"
)

// terminalModes owns all terminal-private state for one raw TUI lifetime.
// Close is intentionally idempotent so every exit path can safely defer it.
type terminalModes struct {
	mu          sync.Mutex
	out         io.Writer
	file        *os.File
	rawRestore  func()
	pasteActive bool
	closed      bool
}

func newTerminalModes(file *os.File, out io.Writer, rawRestore func()) *terminalModes {
	return &terminalModes{file: file, out: out, rawRestore: rawRestore}
}

func (t *terminalModes) EnablePaste() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.pasteActive {
		return nil
	}
	if _, err := io.WriteString(t.out, bracketedPasteEnable); err != nil {
		return err
	}
	t.pasteActive = true
	return nil
}

func (t *terminalModes) DisablePaste() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.pasteActive {
		return nil
	}
	_, err := io.WriteString(t.out, bracketedPasteDisable)
	t.pasteActive = false
	return err
}

func (t *terminalModes) Suspend() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	if t.pasteActive {
		if _, err := io.WriteString(t.out, bracketedPasteDisable); err != nil {
			return err
		}
		t.pasteActive = false
	}
	if t.rawRestore != nil {
		t.rawRestore()
		t.rawRestore = nil
	}
	return nil
}

func (t *terminalModes) Resume() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("terminal session is closed")
	}
	if t.rawRestore == nil {
		restore, err := enableRaw(t.file)
		if err != nil {
			return err
		}
		t.rawRestore = restore
	}
	if !t.pasteActive {
		if _, err := io.WriteString(t.out, bracketedPasteEnable); err != nil {
			return err
		}
		t.pasteActive = true
	}
	return nil
}

func (t *terminalModes) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	var firstErr error
	if t.pasteActive {
		if _, err := io.WriteString(t.out, bracketedPasteDisable); err != nil {
			firstErr = err
		}
		t.pasteActive = false
	}
	if t.rawRestore != nil {
		t.rawRestore()
		t.rawRestore = nil
	}
	return firstErr
}
