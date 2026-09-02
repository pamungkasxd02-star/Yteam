# OpenCode → Go port plan

## Source of truth

The OpenCode source used for behavioral comparison is external to this
repository. Its package names are reproduced here under `packages/`; no local
developer path is required at runtime.

All paths in this repository are portable. Local source checkouts, user names,
and workstation-specific directories are intentionally excluded from source,
tests, and documentation.

## Phases

1. **Foundation** — `packages/opencode`, `packages/core`, `packages/protocol`,
   `packages/schema`, and `packages/tui`: CLI, config, project root, sessions,
   provider streaming, and REPL.
2. **Agent runtime** — message parts, tool calls, retry, compaction, event
   reconciliation, and session lifecycle.
3. **Tools and safety** — read/list/search/edit tools plus deny-by-default
   permissions.
4. **TUI parity** — home/session routes, prompt editor, autocomplete, model /
   agent / session dialogs, sidebar, footer, and stream rendering.
5. **Integrations** — MCP, LSP, Git, skills, server mode, and plugins.
6. **Parity verification** — fixtures and compatibility tests derived from
   OpenCode's public wire contracts.

## Current verified milestone

The Go foundation currently has working implementations and tests for:

- OpenAI-compatible streaming, including streamed tool-call aggregation;
- per-session agent runner and single-run coordinator;
- durable sessions with rename, fork, delete, compact, and export;
- retry helper for transient provider failures and session compaction/revert
  lifecycle metadata;
- event journal with aggregate sequences and session replay;
- permission allow/deny/ask plus once/always/reject waiting;
- project-safe read/list/glob/grep/write/edit/bash tools;
- HTTP health, status, sessions, messages, context, history, prompt, export,
  event, model, agent, tools, and permission routes;
- MCP stdio and LSP JSON-RPC client foundations;
- Git read-only helpers and `SKILL.md` discovery;
- an OpenCode-shaped Home/Session terminal UI foundation;
- raw-key UTF-8/ANSI input, multiline editing, history, searchable pickers,
  live transcript projection, and interactive permission approval;
- durable session input admission with `queue`/`steer`, promotion, and
  interrupt cancellation;
- Windows, Linux, and macOS cross-builds.

Full UI parity and every OpenCode integration are still future layers. A
package directory or README is not counted as a completed implementation.

Revert currently preserves the OpenCode state contract and lifecycle endpoints
(`stage`, `clear`, `commit`). Actual file restoration requires the separate
OpenCode Snapshot service and is intentionally not represented as a fake
implementation.

## Explicit non-goals

- Copying OpenCode's TypeScript source into this repository.
- Bundling Bun, Node.js, or a JavaScript dependency tree.
- Calling a package complete merely because its directory exists.
