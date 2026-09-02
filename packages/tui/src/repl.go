package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
)

func Run(ctx context.Context, app *runtime.Runtime, in io.Reader, out io.Writer) error {
	// Keep piped input deterministic. Interactive terminals use the same
	// OpenCode-shaped Home/Session state through UI, while this path remains
	// friendly to scripts and tests.
	fmt.Fprintln(out, "YTEAM — OpenCode-compatible terminal client")
	fmt.Fprintln(out, "Type /help for help.")
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "\n> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := normalizeLine(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/exit" || line == "/quit" {
			return nil
		}
		if handled, err := app.Command(ctx, line, out); handled {
			if err != nil {
				fmt.Fprintln(out, "error:", err)
			}
			continue
		}
		if err := app.Prompt(ctx, line, out); err != nil {
			fmt.Fprintln(out, "error:", err)
		}
	}
}
