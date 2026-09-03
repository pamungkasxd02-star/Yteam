package containers

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// ExecutionResult holds stdout, stderr and exit code of a containerized run.
type ExecutionResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// Runner executes isolated processes inside docker/podman or local sandboxes.
type Runner struct {
	Image string
}

func NewRunner(image string) *Runner {
	if image == "" {
		image = "alpine:latest"
	}
	return &Runner{Image: image}
}

func (r *Runner) RunCommand(ctx context.Context, cmdStr string, timeout time.Duration) (ExecutionResult, error) {
	start := time.Now()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cCtx, "docker", "run", "--rm", r.Image, "sh", "-c", cmdStr)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	dur := time.Since(start)

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = 1
	}

	return ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: dur,
	}, err
}
