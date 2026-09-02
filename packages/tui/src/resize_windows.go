//go:build windows

package tui

import "os"

// Windows does not expose SIGWINCH. The main loop still refreshes dimensions
// on every frame; a future console event source can replace this no-op watcher.
func watchTerminalResize() (<-chan os.Signal, func()) {
	return nil, func() {}
}
