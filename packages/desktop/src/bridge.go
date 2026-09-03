package desktop

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// Bridge handles desktop OS integrations (opening URLs in default browser, file explorers, notifications).
type Bridge struct{}

func NewBridge() *Bridge {
	return &Bridge{}
}

func (b *Bridge) OpenBrowser(ctx context.Context, targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", targetURL)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", targetURL)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", targetURL)
	}
	return cmd.Start()
}

func (b *Bridge) OpenFolder(ctx context.Context, folderPath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "explorer", folderPath)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", folderPath)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", folderPath)
	}
	return cmd.Start()
}

func (b *Bridge) Platform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
