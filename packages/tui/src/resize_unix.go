//go:build !windows

package tui

import (
	"os"
	"os/signal"
	"syscall"
)

func watchTerminalResize() (<-chan os.Signal, func()) {
	updates := make(chan os.Signal, 1)
	signal.Notify(updates, syscall.SIGWINCH)
	return updates, func() {
		signal.Stop(updates)
		close(updates)
	}
}
