# Package map

These are the package directories found in the OpenCode checkout and their Go
port status. Names intentionally remain unchanged for navigation and future
compatibility.

| OpenCode package | Go status |
| --- | --- |
| `opencode` | Complete Go entry point with CLI subcommand dispatch & TUI run |
| `core` | Core runtime, durable sessions, snapshot rollback, permissions, builtin tools & web tools |
| `core/src/snapshot` | Portable project capture/diff/restore service |
| `agent` | Full agent matrix (`build`, `plan`, `explore`, `general`, `compaction`, `title`, `summary`) |
| `protocol` | Foundation request/response contracts and streaming types |
| `schema` | Data validation, event definitions, message parts, and question schemas |
| `tui` | Rich terminal UI: ANSI styles, Markdown rendering, live diff, fuzzy picker, keymaps |
| `server` | HTTP REST API and SSE real-time streaming endpoint |
| `opencode/src/mcp` | MCP stdio & remote HTTP/SSE client with capability gating and pagination |
| `opencode/src/lsp` | Native LSP JSON-RPC client (diagnostics, hover, definition, code actions) |
| `plugin` | Subprocess JSON-RPC plugin lifecycle and tool bridge |
| `client` | Typed HTTP/SSE SDK for server, session, and event stream contracts |
| `command` | Markdown command registry, hints, and `$1`–`$9`/`$ARGUMENTS` expansion |
| `skill` | `SKILL.md` discovery and skill-command metadata |
| `llm` | Native multi-provider engine: Anthropic, Gemini, Ollama, OpenRouter router |
| `cli` | Full CLI subcommands: `run`, `serve`, `models`, `session`, `export`, `auth`, `version` |
| `stats` | Persistent analytics, token tracking, tool invocation counters |
| `sdk` | High-level programmatic Go SDK for embedding runtime |
| `identity` | Local token store, TTL expiration, and session credentials manager |
| `enterprise` | Corporate RBAC policy engine, model whitelists, and tool boundaries |
| `codemode` | Safe Go AST parser and code refactoring engine |
| `function` | Serverless tool and callable function execution registry |
| `web` | Responsive HTML/CSS web frontend UI endpoint |
| `session-ui` | Presentation view models for TUI and Web interfaces |
| `containers` | Isolated sandbox process runner (Docker-compatible) |
| `desktop` | OS desktop bridge (browser launcher, file manager, notifications) |
