package tui

import "testing"

func TestClipboardCommandsArePlatformSpecificAndPortable(t *testing.T) {
	if got := clipboardCommands("windows"); len(got) == 0 || got[0][0] != "powershell.exe" {
		t.Fatalf("windows clipboard command = %#v", got)
	}
	if got := clipboardCommands("darwin"); len(got) == 0 || got[0][0] != "pbpaste" {
		t.Fatalf("darwin clipboard command = %#v", got)
	}
	if got := clipboardCommands("linux"); len(got) != 3 {
		t.Fatalf("linux clipboard commands = %#v", got)
	}
}
