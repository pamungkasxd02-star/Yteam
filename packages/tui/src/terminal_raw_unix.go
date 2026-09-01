//go:build !windows

package tui

import (
	"os"
	"os/exec"
	"strings"
)

func enableRaw(file *os.File) (func(), error) {
	state, err := terminalCommand(file, "-g")
	if err != nil {
		return nil, err
	}
	if _, err := terminalCommand(file, "-icanon", "-echo"); err != nil {
		return nil, err
	}
	return func() { _, _ = terminalCommand(file, state) }, nil
}

func terminalCommand(file *os.File, args ...string) (string, error) {
	command := exec.Command("stty", args...)
	command.Stdin = file
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}
