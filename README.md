# YTEAM — OpenCode in Go

YTEAM is a lightweight Go reimplementation of OpenCode. OpenCode's original
package and folder names are preserved under `packages/`, while the runtime is
written in Go instead of Bun/TypeScript.

This repository is being ported in layers. The first layer is deliberately
small and runnable: project discovery, configuration, durable sessions,
OpenAI-compatible streaming, Indonesian CLI text, and an interactive REPL.
Later layers will add the agent loop, tools and permissions, TUI, MCP, LSP,
server mode, integrations, and plugin compatibility.

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

Provider settings are read from the environment:

```text
YTEAM_API_KEY=your-key
YTEAM_BASE_URL=https://opencode.ai/zen/v1
YTEAM_MODEL=mimo-v2.5-free
YTEAM_HOME=/path/to/yteam-data
```

The default Zen public marker is only sent in memory. Secrets are not committed
or written to the repository.

## Porting rule

OpenCode is the behavioral reference. Go code must preserve its observable
contracts, but it must not mechanically copy TypeScript implementation files.
Each port layer gets a focused test and a short compatibility note before the
next layer is started.
