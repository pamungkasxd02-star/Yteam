//go:build windows

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

type coord struct {
	x int16
	y int16
}

type smallRect struct {
	left   int16
	top    int16
	right  int16
	bottom int16
}

type consoleScreenBufferInfo struct {
	size              coord
	cursor            coord
	attributes        uint16
	window            smallRect
	maximumWindowSize coord
}

var getConsoleScreenBufferInfo = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleScreenBufferInfo")

func terminalSize(file *os.File) (int, int) {
	if file == nil {
		return 80, 24
	}
	var info consoleScreenBufferInfo
	result, _, _ := getConsoleScreenBufferInfo.Call(uintptr(file.Fd()), uintptr(unsafe.Pointer(&info)))
	if result == 0 || info.window.right < info.window.left || info.window.bottom < info.window.top {
		return 80, 24
	}
	return int(info.window.right - info.window.left + 1), int(info.window.bottom - info.window.top + 1)
}
