package git

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
)

type Status struct {
	Branch    string `json:"branch"`
	Porcelain string `json:"porcelain"`
}

func Read(ctx context.Context, directory string) (Status, error) {
	branch, err := command(ctx, directory, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return Status{}, err
	}
	porcelain, err := command(ctx, directory, "status", "--short")
	if err != nil {
		return Status{}, err
	}
	return Status{Branch: strings.TrimSpace(branch), Porcelain: porcelain}, nil
}

func Diff(ctx context.Context, directory string) (string, error) {
	return command(ctx, directory, "diff", "--no-ext-diff", "--no-color")
}
func Log(ctx context.Context, directory string, count int) (string, error) {
	if count <= 0 {
		count = 10
	}
	return command(ctx, directory, "log", "--oneline", "-n", stringInt(count))
}

func command(ctx context.Context, directory string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = directory
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}
func stringInt(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [24]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
