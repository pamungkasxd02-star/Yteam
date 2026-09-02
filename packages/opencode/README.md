# `opencode`

OpenCode-compatible executable package implemented in Go under `src`.

The `src/mcp` subtree includes stdio plus remote MCP transport foundations:
Streamable HTTP/SSE fallback, paginated tool discovery, duplicate-cursor
protection, and external tool calls.

The core runtime also records a portable pre-prompt project snapshot. Revert
can use that snapshot to restore files and directories, while keeping snapshot
storage outside the project and excluding `.git` metadata.
