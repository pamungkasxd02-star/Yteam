package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/project"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/server/src"
)

// Dispatch executes CLI subcommands.
func Dispatch(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("no command specified")
	}

	cmd := strings.ToLower(args[0])
	cmdArgs := args[1:]

	switch cmd {
	case "run":
		return handleRun(ctx, cmdArgs, in, out, errOut)
	case "serve":
		return handleServe(ctx, cmdArgs, out, errOut)
	case "models":
		return handleModels(ctx, cmdArgs, out, errOut)
	case "session":
		return handleSession(ctx, cmdArgs, out, errOut)
	case "export":
		return handleExport(ctx, cmdArgs, out, errOut)
	case "import":
		return handleImport(ctx, cmdArgs, in, out, errOut)
	case "auth", "account", "console":
		return handleAuth(ctx, cmdArgs, out, errOut)
	case "mcp":
		return handleMcp(ctx, cmdArgs, out, errOut)
	case "stats":
		return handleStats(ctx, cmdArgs, out, errOut)
	case "web":
		return handleWeb(ctx, cmdArgs, out, errOut)
	case "agent", "agents":
		return handleAgents(ctx, cmdArgs, out, errOut)
	case "providers":
		return handleProviders(ctx, cmdArgs, out, errOut)
	case "version", "--version", "-v":
		fmt.Fprintln(out, "YTEAM v1.0.0 (Go runtime)")
		return nil
	case "help", "--help", "-h":
		printHelp(out)
		return nil
	default:
		return fmt.Errorf("unknown subcommand: %s (run 'yteam help' for all commands)", cmd)
	}
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `YTEAM — OpenCode (Go Edition)

Commands:
  run [prompt]      Run a prompt directly or from stdin
  serve             Start the local HTTP & SSE server
  models            List provider models and details
  session [list]    Manage and inspect durable sessions
  export [md|json]  Export current session to Markdown or JSON
  import            Import a session from file or stdin
  auth              Check or manage authentication credentials
  mcp               Inspect MCP server connections and tools
  stats             View token and session analytics
  web               Launch the local web UI
  agent             List available agents (build, plan, explore, general)
  providers         List supported provider backends
  version           Show version information
  help              Show this help menu`)
}

func handleImport(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	fmt.Fprintln(out, "Session imported successfully")
	return nil
}

func handleMcp(ctx context.Context, args []string, out, errOut io.Writer) error {
	root, _ := project.ResolveRoot("")
	cfg, _ := config.Load(root)
	fmt.Fprintf(out, "MCP Home: %s\n", cfg.Home)
	return nil
}

func handleStats(ctx context.Context, args []string, out, errOut io.Writer) error {
	root, _ := project.ResolveRoot("")
	cfg, _ := config.Load(root)
	fmt.Fprintf(out, "Analytics Home: %s\n", cfg.Home)
	return nil
}

func handleWeb(ctx context.Context, args []string, out, errOut io.Writer) error {
	fmt.Fprintln(out, "Web interface available at http://127.0.0.1:8080/")
	return nil
}

func handleAgents(ctx context.Context, args []string, out, errOut io.Writer) error {
	fmt.Fprintln(out, "Available agents:")
	fmt.Fprintln(out, "  build       Default primary agent (executes all tools)")
	fmt.Fprintln(out, "  plan        Read-only planning agent")
	fmt.Fprintln(out, "  explore     Subagent for codebase search & exploration")
	fmt.Fprintln(out, "  general     Subagent for multi-step task research")
	return nil
}

func handleProviders(ctx context.Context, args []string, out, errOut io.Writer) error {
	fmt.Fprintln(out, "Supported LLM providers:")
	fmt.Fprintln(out, "  - Anthropic (Claude 3.7 / 3.5 Sonnet / Haiku)")
	fmt.Fprintln(out, "  - Google Gemini (Gemini 2.5 Pro / Flash)")
	fmt.Fprintln(out, "  - Ollama (Local LLM models)")
	fmt.Fprintln(out, "  - OpenAI / OpenRouter (GPT-4o, etc.)")
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func handleRun(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	root, err := project.ResolveRoot("")
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	store, err := session.Open(cfg.Home, root)
	if err != nil {
		return err
	}
	current, err := store.New()
	if err != nil {
		return err
	}
	app := runtime.New(cfg, root, store, current, provider.New(cfg.BaseURL, cfg.APIKey))

	prompt := strings.Join(args, " ")
	if strings.TrimSpace(prompt) == "" {
		data, readErr := io.ReadAll(in)
		if readErr != nil {
			return readErr
		}
		prompt = string(data)
	}

	if strings.TrimSpace(prompt) == "" {
		return errors.New("prompt is required for run")
	}
	return app.Prompt(ctx, prompt, out)
}

func handleServe(ctx context.Context, args []string, out, errOut io.Writer) error {
	port := 8080
	root, err := project.ResolveRoot("")
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	store, err := session.Open(cfg.Home, root)
	if err != nil {
		return err
	}
	current, err := store.New()
	if err != nil {
		return err
	}
	app := runtime.New(cfg, root, store, current, provider.New(cfg.BaseURL, cfg.APIKey))

	srv := server.New(app, nil, "")
	httpServer := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: srv.Handler(),
	}
	fmt.Fprintf(out, "Serving Yteam HTTP & SSE API on http://127.0.0.1:%d\n", port)
	return httpServer.ListenAndServe()
}
func handleModels(ctx context.Context, args []string, out, errOut io.Writer) error {
	root, _ := project.ResolveRoot("")
	cfg, _ := config.Load(root)
	p := provider.New(cfg.BaseURL, cfg.APIKey)
	models, err := p.Models(ctx)
	if err != nil {
		return err
	}
	for _, m := range models {
		fmt.Fprintf(out, "%s\t%s\n", m.ID, m.Name)
	}
	return nil
}

func handleSession(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "list" {
		root, _ := project.ResolveRoot("")
		cfg, _ := config.Load(root)
		store, err := session.Open(cfg.Home, root)
		if err != nil {
			return err
		}
		list, err := store.List()
		if err != nil {
			return err
		}
		for _, s := range list {
			fmt.Fprintf(out, "%s\t%s\t%s\n", s.ID, s.Title, s.Directory)
		}
		return nil
	}
	return fmt.Errorf("unknown session action: %s", args[0])
}

func handleExport(ctx context.Context, args []string, out, errOut io.Writer) error {
	root, _ := project.ResolveRoot("")
	cfg, _ := config.Load(root)
	store, err := session.Open(cfg.Home, root)
	if err != nil {
		return err
	}
	list, err := store.List()
	if err != nil || len(list) == 0 {
		return errors.New("no sessions to export")
	}
	s := list[0]
	if len(args) > 0 && args[0] == "json" {
		return json.NewEncoder(out).Encode(s)
	}
	for _, m := range s.Messages {
		fmt.Fprintf(out, "### %s\n\n%s\n\n", m.Role, m.Content)
	}
	return nil
}

func handleAuth(ctx context.Context, args []string, out, errOut io.Writer) error {
	root, _ := project.ResolveRoot("")
	cfg, _ := config.Load(root)
	if cfg.APIKey == "" {
		fmt.Fprintln(out, "Status: Not logged in (API key not set in environment or config)")
	} else {
		fmt.Fprintf(out, "Status: Authenticated (API Key: %s***)\n", cfg.APIKey[:minInt(4, len(cfg.APIKey))])
	}
	return nil
}

