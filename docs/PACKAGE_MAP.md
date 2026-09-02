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
| `tui` | Home/Session terminal foundation; full parity staged |
| `server` | core HTTP API and SSE |
| `opencode/src/mcp` | MCP stdio/remote transport, initialization, pagination, and startup config |
| `opencode/src/lsp` | LSP JSON-RPC lifecycle, reusable client selection, and advanced operations |
| `skill` | `SKILL.md` discovery |
| all other `packages/*` | boundary created; implementation staged |
