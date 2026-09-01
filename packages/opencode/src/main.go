package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/event"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/project"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/opencode/src/mcp"
	"github.com/pamungkasxd02-star/Yteam/packages/server/src"
	"github.com/pamungkasxd02-star/Yteam/packages/tui/src"
)

func main() {
	dir := flag.String("dir", "", "direktori proyek")
	model := flag.String("model", "", "ID model")
	sid := flag.String("session", "", "ID session")
	cont := flag.Bool("continue", false, "lanjutkan session terakhir")
	serve := flag.Int("serve", 0, "jalankan server lokal pada port ini")
	serverToken := flag.String("server-token", "", "token server lokal")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, "YTEAM — agen pengembangan Go ringan"); flag.PrintDefaults() }
	flag.Parse()
	root, err := project.ResolveRoot(*dir)
	if err != nil {
		fail(err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		fail(err)
	}
	if *model != "" {
		cfg.Model = *model
	}
	store, err := session.Open(cfg.Home, root)
	if err != nil {
		fail(err)
	}
	current, err := selectSession(store, *sid, *cont)
	if err != nil {
		fail(err)
	}
	app := runtime.New(cfg, root, store, current, provider.New(cfg.BaseURL, cfg.APIKey))
	journal, err := event.Open(cfg.Home)
	if err != nil {
		fail(err)
	}
	app.AttachEvents(journal)
	mcpManager := mcp.NewManager()
	app.SetMCPStatus(func() any { return mcpManager.Status() })
	_, _ = app.Skills()
	if *serve > 0 {
		srv := server.New(app, journal, *serverToken)
		address := fmt.Sprintf("127.0.0.1:%d", *serve)
		fmt.Fprintln(os.Stderr, "server YTEAM berjalan di http://"+address)
		if err := http.ListenAndServe(address, srv.Handler()); err != nil {
			fail(err)
		}
		return
	}
	message := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if message != "" {
		if err := app.Prompt(context.Background(), message, os.Stdout); err != nil {
			fail(err)
		}
		return
	}
	if !terminal(os.Stdin) {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail(err)
		}
		if strings.TrimSpace(string(data)) != "" {
			if err := app.Prompt(context.Background(), string(data), os.Stdout); err != nil {
				fail(err)
			}
		}
		return
	}
	if err := tui.New(app, os.Stdin, os.Stdout).Run(context.Background()); err != nil {
		fail(err)
	}
}
func selectSession(store *session.Store, id string, cont bool) (*session.Session, error) {
	if id != "" {
		return store.Load(id)
	}
	if cont {
		return store.Latest()
	}
	return store.New()
}
func terminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
func fail(err error) { fmt.Fprintln(os.Stderr, "gagal:", err); os.Exit(1) }
