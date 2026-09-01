package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
)

func Run(ctx context.Context, app *runtime.Runtime, in io.Reader, out io.Writer) error {
	// Keep piped input deterministic. Interactive terminals use the same
	// OpenCode-shaped Home/Session state through UI, while this path remains
	// friendly to scripts and tests.
	fmt.Fprintln(out, "YTEAM — OpenCode portado para Go")
	fmt.Fprintln(out, "Ketik /help untuk bantuan.")
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "\n> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if handled, err := app.Command(ctx, line, out); handled {
			if err != nil {
				fmt.Fprintln(out, "error:", err)
			}
			continue
		}
		switch line {
		case "/exit", "/quit":
			return nil
		default:
			if err := app.Prompt(ctx, line, out); err != nil {
				fmt.Fprintln(out, "error:", err)
			}
		}
	}
}
