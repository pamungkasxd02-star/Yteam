package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

func readSystemClipboard() (string, error) {
	commands := clipboardCommands(runtime.GOOS)
	if len(commands) == 0 {
		return "", fmt.Errorf("system clipboard is not supported on %s", runtime.GOOS)
	}
	var lastErr error
	for _, item := range commands {
		output, err := exec.CommandContext(context.Background(), item[0], item[1:]...).Output()
		if err == nil {
			return string(output), nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("unable to read system clipboard: %w", lastErr)
}

func clipboardCommands(goos string) [][]string {
	switch goos {
	case "windows":
		return [][]string{{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard -Raw"}}
	case "darwin":
		return [][]string{{"pbpaste"}}
	default:
		return [][]string{{"wl-paste", "--no-newline"}, {"xclip", "-selection", "clipboard", "-out"}, {"xsel", "--clipboard", "--output"}}
	}
}
