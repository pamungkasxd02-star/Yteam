# YTEAM (OpenCode in Go)

High-performance, lightweight pure Go implementation of OpenCode. Built to eliminate Node/Bun/V8 overhead while maintaining full protocol, TUI, and integration parity with OpenCode.

## Overview

YTEAM provides the full OpenCode terminal and headless developer agent experience in a single static binary:
- **Low Memory Footprint**: ~15–25 MB RAM consumption (zero V8/Electron bloat).
- **Full Cross-Platform**: Native Linux, macOS, and Windows support without external runtime dependencies.
- **Multi-Provider Engine**: Native adapters for Anthropic Claude (Messages API), Google Gemini (REST SSE), Ollama, and OpenAI/OpenRouter backends.
- **Built-in Free Tier**: Out-of-the-box support for OpenCode Zen free models (`mimo-v2.5-free`, `gemini-2.5-flash`, `deepseek-v3-free`, `llama-3.3-70b-free`, etc.).
- **Rich Terminal UI**: Live thinking/reasoning stream viewer, code syntax highlighting, fuzzy finder pickers, split-pane job monitor, live git diff inspector, and `@`/`/` autocomplete triggers.
- **Integrations**: Full support for Model Context Protocol (MCP stdio & remote), Language Server Protocol (LSP), JSON-RPC plugins, and Markdown skill definitions.

---

## Installation & Build

### Prerequisites
- Go 1.22+

### Build from source

**Linux / macOS:**
```bash
git clone https://github.com/pamungkasxd02-star/Yteam.git
cd Yteam
go build -trimpath -o yteam ./packages/opencode/src
```

**Windows (PowerShell / Command Prompt):**
```powershell
git clone https://github.com/pamungkasxd02-star/Yteam.git
cd Yteam
go build -trimpath -o yteam.exe .\packages\opencode\src
```

---

## Usage

### Interactive TUI Mode
Start an interactive terminal session in the current directory:

**Windows (CMD / Command Prompt):**
```cmd
yteam.exe
```

**Windows (PowerShell):**
```powershell
.\yteam.exe
```

**Linux / macOS:**
```bash
./yteam
```

Start in a specific project with custom agent/model:
```bash
yteam.exe -dir ./my-project -model mimo-v2.5-free -agent build
```

### CLI Subcommands
Run headless commands, start servers, or manage sessions:

```cmd
:: Windows CMD / PowerShell / Linux
yteam.exe run "Analyze the project structure and suggest improvements"
yteam.exe serve
yteam.exe models
yteam.exe session list
yteam.exe export md
yteam.exe mcp
yteam.exe stats
yteam.exe help
```

---

## Interactive TUI Commands

Inside the terminal UI:
- `/models`: Open fuzzy-search model selector.
- `/agents`: Switch agent mode (`build`, `plan`, `explore`, `general`).
- `/sessions`: Switch, continue, or resume sessions.
- `/new` (or `/clear`): Create a fresh session.
- `/fork`: Fork current conversation into a new branch.
- `/diff`: View live git working tree changes.
- `/git`: Check git branch and status.
- `/stash`: Stash or pop current prompt drafts.
- `/mcps`: View active MCP servers and registered tools.
- `/lsp`: Inspect active Language Server Protocol clients.
- `/help`: Display in-terminal command palette.
- `/exit`: Exit the application.

---

## Configuration

Settings are resolved hierarchically without hardcoded machine paths:
1. `<YTEAM_HOME>/config.json` (or `~/.config/yteam/config.json`)
2. Project `yteam.json` / `.yteam.json`
3. Environment variables (e.g. `YTEAM_API_KEY`, `YTEAM_MODEL`, `YTEAM_BASE_URL`)

### Example Environment Variables

```bash
# OpenCode Zen / Free Tier endpoint (Default)
export YTEAM_BASE_URL="https://opencode.ai/zen/v1"
export YTEAM_MODEL="mimo-v2.5-free"
export YTEAM_API_KEY="your-api-key"

# Custom Anthropic or Ollama settings
export YTEAM_MODEL="claude-3-7-sonnet"
# or
export YTEAM_MODEL="ollama/qwen2.5-coder"
```

---

## Testing

Run the full cross-platform test suite:

```bash
go test ./...
```

---

## License

MIT License.
