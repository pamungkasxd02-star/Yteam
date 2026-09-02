package tui

import (
	"bytes"
	"testing"
)

func TestTerminalModesEnableDisablePasteIsIdempotent(t *testing.T) {
	var output bytes.Buffer
	restores := 0
	modes := newTerminalModes(nil, &output, func() { restores++ })
	if err := modes.EnablePaste(); err != nil {
		t.Fatal(err)
	}
	if err := modes.EnablePaste(); err != nil {
		t.Fatal(err)
	}
	if err := modes.DisablePaste(); err != nil {
		t.Fatal(err)
	}
	if err := modes.DisablePaste(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), bracketedPasteEnable+bracketedPasteDisable; got != want {
		t.Fatalf("terminal modes = %q, want %q", got, want)
	}
	if err := modes.Close(); err != nil {
		t.Fatal(err)
	}
	if err := modes.Close(); err != nil {
		t.Fatal(err)
	}
	if restores != 1 {
		t.Fatalf("restore count = %d, want 1", restores)
	}
}

func TestTerminalModesCloseDisablesPasteBeforeRestore(t *testing.T) {
	var output bytes.Buffer
	modes := newTerminalModes(nil, &output, func() { output.WriteString("restore") })
	if err := modes.EnablePaste(); err != nil {
		t.Fatal(err)
	}
	if err := modes.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), bracketedPasteEnable+bracketedPasteDisable+"restore"; got != want {
		t.Fatalf("cleanup order = %q, want %q", got, want)
	}
}

func TestTerminalModesSuspendAndResumeOwnPasteMode(t *testing.T) {
	var output bytes.Buffer
	restores := 0
	modes := newTerminalModes(nil, &output, func() { restores++ })
	if err := modes.EnablePaste(); err != nil {
		t.Fatal(err)
	}
	if err := modes.Suspend(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), bracketedPasteEnable+bracketedPasteDisable; got != want {
		t.Fatalf("suspend output = %q, want %q", got, want)
	}
	if restores != 1 {
		t.Fatalf("suspend restore count = %d, want 1", restores)
	}
}
