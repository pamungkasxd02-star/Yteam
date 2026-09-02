//go:build !windows

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

type terminalWindowSize struct {
	rows uint16
	cols uint16
}

func terminalSize(file *os.File) (int, int) {
	if file == nil {
		return 80, 24
	}
	var size terminalWindowSize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&size)))
	if errno != 0 || size.cols == 0 || size.rows == 0 {
		return 80, 24
	}
	return int(size.cols), int(size.rows)
}
