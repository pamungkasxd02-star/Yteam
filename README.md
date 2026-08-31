# YTEAM

YTEAM is a local-first security-research workbench for authorized testing. It
combines a terminal agent UI, a small durable runtime, explicit scope and
safety policy, bounded recon, browser observation, and local evidence/state
storage.

The project is self-contained. It does not require Bun, an upstream agent
runtime, a vendored source tree, or a global installation. Runtime data stays
under the ignored `runtime/` directory; source code, tests, and reviewed
first-party skills are kept in the repository.

> **Authorized use only.** Use YTEAM only against assets covered by written
> permission, such as a HackerOne, Bugcrowd, Intigriti, QA, or internal test
> engagement. The default workflow is read-only, low-rate, scoped, and does not
> submit reports automatically.

## What is included

- Full-screen OpenCode-style TUI built with `prompt_toolkit`.
- OpenAI-compatible streaming client with a keyless Zen Free default.
- SQLite WAL state for sessions, messages, events, and durable jobs.
- Fail-closed target validation and local runtime policy.
- Bounded read-only recon pipeline with evidence and non-claim tracking.
- LocalSolver: allowlisted browser observation using Camoufox.
- Native stdio MCP server exposing read-only YTEAM tools.
- First-party skill registry with risk classification and on-demand loading.
- Durable worker with leases, heartbeat, stale-job recovery, and retries.
- Composable engine: policy, DAG graph, scheduler, adaptive planner, knowledge
  graph, skill resolver, and context guard.
- Local redaction, secret scanning, stream filtering, and report aggregation.

## Install

### Windows PowerShell

```powershell
git clone https://github.com/pamungkasxd02-star/Yteam.git Yteam
Set-Location Yteam
python scripts\install_yteam.py
```

### macOS/Linux

```bash
git clone https://github.com/pamungkasxd02-star/Yteam.git yteam
cd yteam
python3 scripts/install_yteam.py
```

The installer creates `runtime/.venv`, installs the pinned packages from
`requirements.txt`, optionally downloads Camoufox browser data into the
runtime cache, and creates user-local launchers for:

```text
yteam          TUI
yteam-control  signed local remote-control adapter
yteam-worker   durable assessment worker
localsolver    browser-observation service
yteam-mcp      read-only MCP server
```

Each launcher calls the Python interpreter inside this checkout's
`runtime/.venv`; it does not depend on whichever global Python happens to be on
`PATH`. The installer also persists the launcher directory in the user's PATH
for future PowerShell, cmd, bash, zsh, or sh sessions. A running shell cannot
receive environment changes from a child installer process, so Windows prints
an immediate absolute-path command and a one-line PATH refresh command at the
end of installation. Run the printed absolute-path command immediately, or
refresh the current PowerShell with the printed `$env:Path = ...` line. No
administrator shell is required.

Preview the installation without changing files:

```text
python scripts/install_yteam.py --dry-run
```

Skip only browser-data download when the browser is already available or when
installing in CI:

```text
python scripts/install_yteam.py --skip-browser-download
```

Python dependencies are pinned in `requirements.txt`. On Linux, Camoufox may
also need system browser libraries; see `REQUIREMENTS.md`.

## TUI

Start the default full-screen interface:

```text
yteam
```

The initial state is a centered composer:

```text
                 YTEAM

        ┌─────────────────────────────┐
        │ Ask anything...              │
        │ Bb auto · <model> YTEAM      │
        └─────────────────────────────┘
             tab agents  ctrl+p commands
```

After the first message, it becomes a workspace with the transcript on the
left and a persistent information rail on the right:

```text
┌──────────────────────────────────────────────┬─────────────────────┐
│                                              │ YTEAM Security Agent│
│              session transcript              │ Context             │
│      prompts, streamed answers, and logs      │ MCP status          │
│                                              │ Memory              │
│  ▌ Ask anything...                           │ working directory   │
│    Bb auto · <model> YTEAM                   │ version             │
│  esc interrupt              ctrl+p commands  │                     │
└──────────────────────────────────────────────┴─────────────────────┘
```

The visual style is implemented independently; no upstream UI source is
required. Controls:

| Key | Action |
|---|---|
| `Enter` | Submit the current prompt |
| `Ctrl+J` | Insert a newline in the composer |
| `Arrow Up/Down` | Navigate prompt history |
| `Ctrl+P` | Open the command palette |
| `/` | Start slash-command completion |
| `PageUp/PageDown` | Move transcript scroll position |
| `Escape` | Interrupt an active model stream |
| `Ctrl+C` | Exit |

For pipes, CI, or a terminal without interactive support:

```text
python scripts/yteam_tui.py --plain
```

## Runtime commands

Inside the TUI:

| Command | Purpose |
|---|---|
| `/help` | Show commands |
| `/models` | Discover the available Zen Free models |
| `/model <id>` | Select a model |
| `/status` | Show runtime, policy, session, engine, and job state |
| `/history` | Show recent conversation messages |
| `/clear` | Start a fresh local session |
| `/memory` | Show verified lessons and pending proposals |
| `/learn <text>` | Store a redacted lesson proposal |
| `/verify <id>` | Promote a proposal to verified memory |
| `/events` | Show replayable runtime events |
| `/jobs` | Show durable assessment jobs |
| `/skills` | Show skill count and risk summary |
| `/engine` | Show engine policy, planner, scheduler, graph, and cache state |
| `/plan <target>` | Generate a policy-bound adaptive plan |
| `/ctx` | Show context usage, compaction, and handoff state |
| `/doctor` | Run local diagnostics |
| `/bb <target>` | Queue a scoped read-only assessment |
| `/quit` | Exit |

## Durable assessment worker

`/bb` writes a job to `runtime/state.db`. The worker claims jobs with a lease,
updates a heartbeat, checkpoints the pipeline ledger, retries transient errors,
and recovers stale jobs after a process failure.

```text
yteam-worker
python scripts/yteam_worker.py --once
```

The worker does not submit reports, access customer objects, or enable
destructive actions. The job output and pipeline records remain local.

## Context guard

Long conversations are protected by `src/yteam_engine/context_guard.py`:

- below 75% of the configured context window: normal operation;
- at 75%: the oldest turns are folded into a prompt-side summary;
- at 85%: a Markdown handoff is written to `runtime/handoffs/` with a
  continuation command for a fresh session.

The SQLite message history is not deleted. Compaction only changes what is sent
to the model. Use `/ctx` to inspect the current estimate.

## Model configuration

The default configuration is keyless Zen Free:

```yaml
provider: zen-free
model: laguna-s-2.1-free
api_key: ""
base_url: "https://opencode.ai/zen/v1"
```

Copy `yteam.local.example.yaml` to `yteam.local.yaml` for a local override.
The override is ignored by Git. YTEAM sends standard OpenAI-compatible
`POST /chat/completions` requests with SSE streaming.

## LocalSolver

LocalSolver is an allowlisted browser-observation service for authorized recon.
It classifies responses and gate metadata through an asynchronous task queue;
it is not a CAPTCHA solver, proxy rotator, WAF bypass, credential tool, or
token/cookie exporter.

```powershell
$env:LOCALSOLVER_TARGET_ALLOWLIST = "https://authorized-target.example"
$env:LOCALSOLVER_API_KEY = "local-secret"
localsolver --host 127.0.0.1 --port 8001 --workers 2
```

Endpoints:

```text
GET  /health
POST /observe   {"url":"https://authorized-target.example","headless":true}
GET  /result?id=<task_id>
GET  /tasks
```

## Skills and MCP

The repository currently ships five reviewed first-party skills:

| Skill | Focus |
|---|---|
| `yteam-recon` | Scope-aware, bounded surface mapping |
| `yteam-authorization` | IDOR/BOLA and role/tenant boundaries |
| `yteam-injection` | Safe input-boundary canaries |
| `yteam-reporting` | Evidence, triage, and report quality |
| `yteam-runtime` | Runtime, state, policy, and worker operation |

Skill files live under `skills/<name>/SKILL.md`; `skills/catalog.json` is the
portable metadata index. YTEAM does not download or require an external skill
corpus.

Start the read-only MCP server with:

```text
yteam-mcp
```

It exposes skill metadata/loading, scope validation, stream filtering, masked
secret scanning, report aggregation, engine status, and plan generation. Active
network assessment remains in the durable worker.

## Architecture

```text
scripts/yteam_tui.py       full-screen TUI and plain fallback
scripts/yteam_runtime.py   commands, model/session lifecycle, events
scripts/yteam_state.py     SQLite WAL state and durable jobs
scripts/yteam_memory.py    verified two-phase learning memory
scripts/yteam_worker.py    lease-based assessment worker
scripts/yteam_recon.py     bounded web recon pipeline
scripts/localsolver.py     browser-observation HTTP service
scripts/yteam_mcp.py       read-only MCP server
src/local_solver/          gate detector, browser adapter, queue, service
src/yteam_engine/          policy, DAG, scheduler, planner, graph, context
skills/                    reviewed first-party playbooks and catalog
```

## Development

```powershell
python scripts/yteam_doctor.py --json
python -m compileall -q scripts src
python -m unittest discover -s tests -p "test_*.py" -v
```

The test suite covers the runtime, state store, policy gates, LocalSolver,
engine modules, context guard, and both TUI visual states.

Runtime databases, browser caches, job output, reports, evidence, credentials,
and local model configuration are ignored by Git. Do not commit cookies,
bearer tokens, customer data, or unredacted evidence.

See `REQUIREMENTS.md`, `YTEAM_SECURITY.md`, `SECURITY.md`, and
`docs/ARCHITECTURE.md` for operational details.
