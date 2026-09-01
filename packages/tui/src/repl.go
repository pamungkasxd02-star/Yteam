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
		switch line {
		case "/exit", "/quit":
			return nil
		case "/help":
			app.Help(out)
		case "/status":
			app.Status(out)
		case "/history":
			app.History(out)
		case "/clear":
			next, err := app.Store.New()
			if err != nil {
				return err
			}
			app.Session = next
			fmt.Fprintln(out, "Session baru:", next.ID)
		case "/models":
			models, err := app.Provider.Models(ctx)
			if err != nil {
				fmt.Fprintln(out, "gagal mengambil model:", err)
				continue
			}
			for _, model := range models {
				fmt.Fprintln(out, model.ID)
			}
		default:
			if err := app.Prompt(ctx, line, out); err != nil {
				fmt.Fprintln(out, "error:", err)
			}
		}
	}
}
