# Package map

These are the package directories found in the OpenCode checkout and their Go
port status. Names intentionally remain unchanged for navigation and future
compatibility.

| OpenCode package | Go status |
| --- | --- |
| `opencode` | foundation executable |
| `core` | foundation runtime, provider catalog, and usage accounting |
| `core/src/snapshot` | portable project capture/diff/restore service |
| `agent` | built-in build/plan catalog |
| `protocol` | foundation contracts |
| `schema` | foundation data types |
| `tui` | English Home/Session terminal UI with prompt history, parts, input display-width handling, viewport, dialogs, and staged OpenCode parity |
| `server` | core HTTP API and SSE |
| `opencode/src/mcp` | MCP stdio/remote transport, initialization, pagination, and startup config |
| `opencode/src/lsp` | LSP JSON-RPC lifecycle, reusable client selection, and advanced operations |
| `plugin` | portable subprocess JSON-RPC plugin lifecycle and tool bridge |
| `client` | typed HTTP/SSE SDK for server and session contracts |
| `command` | Markdown command registry, hints, and template expansion |
| `skill` | `SKILL.md` discovery and skill-command metadata |
| all other `packages/*` | boundary created; implementation staged |
