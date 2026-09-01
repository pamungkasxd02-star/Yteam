//go:build windows

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	modeProcessedInput = 0x0001
	modeLineInput      = 0x0002
	modeEchoInput      = 0x0004
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var getConsoleMode = kernel32.NewProc("GetConsoleMode")
var setConsoleMode = kernel32.NewProc("SetConsoleMode")

func enableRaw(file *os.File) (func(), error) {
	var mode uint32
	if result, _, err := getConsoleMode.Call(uintptr(file.Fd()), uintptr(unsafe.Pointer(&mode))); result == 0 {
		return nil, err
	}
	raw := mode &^ (modeProcessedInput | modeLineInput | modeEchoInput)
	if result, _, err := setConsoleMode.Call(uintptr(file.Fd()), uintptr(raw)); result == 0 {
		return nil, err
	}
	return func() { _, _, _ = setConsoleMode.Call(uintptr(file.Fd()), uintptr(mode)) }, nil
}
