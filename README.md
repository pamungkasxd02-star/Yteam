# YTEAM — OpenCode in Go

YTEAM is a lightweight Go reimplementation of OpenCode. OpenCode's original
package and folder names are preserved under `packages/`, while the runtime is
written in Go instead of Bun/TypeScript.

This repository is being ported in layers. The first layer is deliberately
small and runnable: project discovery, configuration, durable sessions,
OpenAI-compatible streaming, agent/tool execution, Indonesian CLI text, and an
interactive Home/Session terminal UI. Later layers will add full TUI parity,
MCP/LSP integrations, server breadth, and plugin compatibility.

## Layout

The directory names mirror the OpenCode repository:

```text
packages/
  opencode/          Go executable entry point
  core/              config, project, session, provider runtime
  protocol/          wire-level request/response contracts
  schema/            shared data validation/types
  tui/               terminal interaction layer
  session-ui/        session presentation contracts
  cli/ client/ ...   reserved OpenCode-compatible package boundaries
```

The complete package tree is present even when a package is still marked
`planned`. No Bun, Node.js, TypeScript, or OpenCode source files are vendored.

## Build and test

```bash
go test ./...
go vet ./...
go build -trimpath -o yteam ./packages/opencode/src
```

Windows PowerShell:

```powershell
go build -trimpath -o yteam.exe .\packages\opencode\src
```

## Run

```text
yteam /help
yteam /status
yteam
```

Interactive commands include `/models`, `/model <id>`, `/agents`,
`/agent <name>`, `/sessions`, `/new`, `/fork`, `/rename <title>`, and
`/export [md|json]`.

Use `-agent plan` or `YTEAM_AGENT=plan` to run the read-only planning agent;
`build` remains the default and can use the full tool set.

Session API lifecycle also includes compaction and revert state. Revert metadata
and portable pre-prompt file restoration are available through the snapshot
service.

The interactive terminal uses raw ANSI/UTF-8 input when attached to a TTY:
multiline text uses `Ctrl+J`, history uses `Up`/`Down`, pickers support
`Up`/`Down`/`Enter`/`Esc`, and permission prompts accept `y` (once), `a`
(always), or `n` (reject). Piped input keeps the line-oriented REPL behavior.

Provider settings are read from the environment:

```text
YTEAM_API_KEY=your-key
YTEAM_BASE_URL=https://opencode.ai/zen/v1
YTEAM_MODEL=mimo-v2.5-free
YTEAM_HOME=/path/to/yteam-data
```

Configuration files are merged in this order: `<YTEAM_HOME>/config.json`,
project `yteam.json`, project `.yteam.json`, project `.yteam/config.json`,
then optional `YTEAM_CONFIG`. Environment variables override all files, and
CLI flags such as `-model` and `-agent` override the environment for that run.
This hierarchy is portable and uses no checkout-specific paths.

Remote MCP servers can be configured in `mcp.json` below `YTEAM_HOME`:

```json
{
  "servers": {
    "docs": {
      "URL": "https://example.invalid/mcp",
      "Headers": {"Authorization": "Bearer token"},
      "Timeout": 30000000000
    }
  }
}
```

For a single CI/container server, use `YTEAM_MCP_URL`, optional JSON
`YTEAM_MCP_HEADERS`, and `YTEAM_MCP_TIMEOUT` such as `30s`. Remote MCP
connections initialize and discover paginated tools during startup; failures
are exposed in `/api/mcp` and do not prevent the CLI from starting.

Plugins use a portable subprocess JSON-RPC bridge. Configure `plugins.json`
below `YTEAM_HOME` or set `YTEAM_PLUGIN_CONFIG`; each plugin is initialized,
its tools are discovered, and failures are isolated in `/api/plugin`.

The provider model catalog is cached after `/models` discovery. Model metadata
can include variants and token pricing; completion usage is exposed through
`/api/provider/usage`. The default Zen public marker is only sent in memory.
Secrets are not committed or written to the repository.

Session metadata records the current run lifecycle (`busy`, `retrying`,
`completed`, `failed`, or `interrupted`) together with retry/error and timing
information. The same transitions are emitted as durable events for API/SSE
clients and the terminal transcript reducer.

Compaction stores a monotonically increasing context epoch plus provider-
independent token estimates before and after summarization. The estimates are
for budgeting and observability, not a replacement for a provider tokenizer.

Question prompts are persisted below `YTEAM_HOME/questions.jsonl`. Pending
questions are replayed after restart, while replies and rejections remain
available to a waiting tool even when the answer arrives before `Await` starts.

Permission prompts use the same durable approach in
`YTEAM_HOME/permissions.jsonl`. Pending approvals survive restart, terminal
replies are retained for waiting tools, and `Always` approvals replay as
allow-rules without weakening the default deny/ask behavior.

The verified portable build targets are Windows, Linux amd64, and macOS amd64.
Use `go test -p 1 ./...` on constrained Windows/386 environments to avoid
parallel compiler pagefile pressure.

## Porting rule

OpenCode is the behavioral reference. Go code must preserve its observable
contracts, but it must not mechanically copy TypeScript implementation files.
Each port layer gets a focused test and a short compatibility note before the
next layer is started.
