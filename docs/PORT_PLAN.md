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
- OpenAI-compatible model catalog metadata, model variants, finish reasons,
  response usage accounting, and provider usage API;
- per-session agent runner and single-run coordinator;
- durable sessions with rename, fork, delete, compact, and export;
- retry helper for transient provider failures and session compaction/revert
  lifecycle metadata;
- portable project snapshots with deterministic file diffs, checksums, safe
  restore, and rollback-on-failure wired into revert commit;
- event journal with aggregate sequences and session replay;
- permission allow/deny/ask plus once/always/reject waiting;
- project-safe read/list/glob/grep/write/edit/bash tools;
- HTTP health, status, sessions, messages, context, history, prompt, export,
  event, model, agent, tools, and permission routes;
- MCP stdio plus remote transport/pagination and LSP JSON-RPC
  client/operation foundations;
- remote MCP initialization and startup configuration from portable
  `mcp.json`/environment sources, with failed connections retained as visible
  status instead of aborting the executable;
- MCP catalog pagination for tools, prompts, resources, and resource templates
  with capability gating and upstream-compatible cursor semantics;
- LSP initialize/initialized lifecycle, reusable root-scoped clients,
  extension-aware client selection, diagnostics, code actions, implementation,
  and call-hierarchy operations;
- portable subprocess plugin bridge with manifest loading, initialize,
  tools/list, tools/call, isolated status, and tool-registry integration;
- typed client SDK with bearer authentication, session/lifecycle operations,
  provider resources, and replayable global/session SSE event streams;
- typed client status/context/error contracts with standard SSE comment,
  multiline-data, close, and EOF handling;
- durable message metadata and typed parts for reasoning, model, usage,
  finish state, tool calls, and tool results, including live event projection;
- durable per-session run lifecycle with `busy`, `retrying`, `completed`,
  `failed`, and `interrupted` state, timestamps, retry attempts, and terminal
  events consumed by TUI/server projections;
- compaction context epochs with deterministic token estimates before/after
  compaction, durable epoch persistence, and `context` API projection;
- durable question requests with replayable pending/terminal state, session
  binding, and reply-before-await handling across process restarts;
- durable permission approvals with replayable pending/terminal state,
  reply-before-await handling, session-safe requests, and persisted `Always`
  rules;
- server/client run-state contract with typed status mapping for missing,
  forbidden, validation, and internal errors;
- agent catalog behavior with persisted selection, agent-specific system
  prompts, CLI/environment selection, and plan-mode read-only tool filtering;
- portable configuration hierarchy with user/project/explicit-file merge,
  environment precedence, CLI overrides, and malformed-file validation;
- command registry with upstream `init`/`review` entries, Markdown
  frontmatter, source metadata, argument hints, `$1`–`$9`/`$ARGUMENTS`
  expansion, discovered skill commands, and runtime/server/client listing;
- usable English TUI prompt worker and persistent prompt history matching
  OpenCode's 50-entry JSONL/dedupe/replay behavior, typed prompt parts, tracked
  paste expansion, and command-first piped input;
- Git read-only helpers and `SKILL.md` discovery;
- an OpenCode-shaped English Home/Session terminal UI foundation;
- raw-key UTF-8/ANSI input, grapheme-aware display offsets, multiline editing,
  bracketed paste, PageUp/PageDown viewport navigation, persistent history,
  searchable pickers, external editor integration, live transcript projection,
  and interactive permission approval;
- durable session input admission with `queue`/`steer`, promotion, and
  interrupt cancellation;
- Windows, Linux, and macOS cross-builds.

Full UI parity and every OpenCode integration are still future layers. A
package directory or README is not counted as a completed implementation.

Revert preserves the OpenCode state contract and lifecycle endpoints (`stage`,
`clear`, `commit`). The Go port now includes a portable Snapshot service under
`packages/core/src/snapshot`: it captures project files outside the project
tree, excludes `.git`, computes deterministic status output, validates
manifests/checksums, and restores the pre-prompt tree on commit. Snapshots are
stored below the configured application home and never use machine-specific
paths from source or documentation.

## Explicit non-goals

- Copying OpenCode's TypeScript source into this repository.
- Bundling Bun, Node.js, or a JavaScript dependency tree.
- Calling a package complete merely because its directory exists.
