# `opencode`

OpenCode-compatible executable package implemented in Go under `src`.

The `src/mcp` subtree includes stdio plus remote MCP transport foundations:
Streamable HTTP/SSE fallback, paginated tool discovery, duplicate-cursor
protection, and external tool calls.

Remote MCP configuration is loaded from `YTEAM_MCP_CONFIG`,
`<YTEAM_HOME>/mcp.json`, or `YTEAM_MCP_URL` during executable startup.

The `src/lsp` subtree supports the initialize/initialized lifecycle, root and
extension-aware client reuse, diagnostics, code actions, implementation, and
call-hierarchy requests in addition to definition, references, hover, and
symbol operations.

The core runtime also records a portable pre-prompt project snapshot. Revert
can use that snapshot to restore files and directories, while keeping snapshot
storage outside the project and excluding `.git` metadata.
