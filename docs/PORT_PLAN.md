# OpenCode → Go port plan

## Source of truth

The audited OpenCode tree is at `D:\opencode\tmp\opencode-upstream`. Its
package names are reproduced in this repository under `packages/`.

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

## Explicit non-goals

- Copying OpenCode's TypeScript source into this repository.
- Bundling Bun, Node.js, or a JavaScript dependency tree.
- Calling a package complete merely because its directory exists.
