# YTEAM

YTEAM is a standalone, local-first security-research workbench. It owns its
native Python terminal UI, policy engine, JSONL session store, append-only event
ledger, model client, skill registry, bounded recon pipeline, hypothesis
planner, evidence hygiene, and multi-pillar assessment DAG.

There is **no required vendored agent runtime, external UI, Bun installation,
or global tool mutation**. YTEAM can use an OpenAI-compatible model endpoint,
including the keyless Zen Free catalog, while keeping local sessions and
assessment artifacts under `runtime/`.

## Install

### Windows

```powershell
git clone https://github.com/pamungkasxd02-star/Yteam.git Yteam
cd Yteam
python scripts\install_yteam.py --skip-browser-download
```

### macOS/Linux

```bash
git clone https://github.com/pamungkasxd02-star/Yteam.git yteam
cd yteam
python3 scripts/install_yteam.py --skip-browser-download
```

The installer creates `runtime/.venv`, installs `requirements.txt`, and places
a user-local `yteam` launcher. Omit `--skip-browser-download` if the optional
Camoufox browser observer is required.

Preview the plan without changing anything:

```text
python scripts/install_yteam.py --dry-run
```

## Native TUI

Start it with:

```text
yteam
```

Commands:

```text
/help                         show commands
/models                       discover/list Zen Free models
/model <model-id>             select a model
/status                       show policy/session/runtime state
/history                      show the current bounded conversation
/clear                        create a fresh local session
/memory                       show verified lessons and pending proposals
/events                       show replayable runtime events
/learn <lesson>               propose a lesson for verification
/verify <proposal-id>         verify a lesson for future prompt context
/doctor                       run local diagnostics
/bb <authorized-http-target>  run the scoped read-only assessment
/quit                         exit
```

`/bb` drives the native pipeline:

```text
scope → inventory → baseline → bounded crawl → route mining
      → skill selection → hidden-surface hypotheses → intelligence
      → evidence manifest → triage handoff
```

The pipeline is fail-closed, low-rate, read-only by default, and never
auto-submits a report. Recon signals, scanner matches, and hypotheses are not
findings until reproducible impact and triage gates pass.

## Model configuration

The default is keyless Zen Free:

```yaml
provider: zen-free
model: laguna-s-2.1-free
api_key: ""
base_url: "https://opencode.ai/zen/v1"
```

To override it, copy `yteam.local.example.yaml` to `yteam.local.yaml`. The
local file is ignored by Git; its API key is held in memory and is never written
to sessions, event logs, or generated assessment artifacts. `/models` refreshes
the live catalog and falls back to a bundled list when the endpoint is offline.

The native client sends standard OpenAI-compatible `POST /chat/completions`
requests with SSE streaming. It does not start a gateway or proxy process.

## Remote control

Remote control is opt-in and local-only by default. The installer also creates
`yteam-control`. It accepts signed webhook commands for Discord and WhatsApp
bridges and can long-poll Telegram when its token and actor allowlist are set.

```powershell
$env:YTEAM_CONTROL_SECRET = "use-a-long-random-local-secret"
$env:YTEAM_TELEGRAM_BOT_TOKEN = "bot-token-from-your-secret-store"
$env:YTEAM_TELEGRAM_ALLOWLIST = "telegram-chat-id"
$env:YTEAM_DISCORD_ALLOWLIST = "discord-user-id"
$env:YTEAM_WHATSAPP_ALLOWLIST = "E.164-sender-id"
$env:YTEAM_REMOTE_TARGET_ALLOWLIST = "https://authorized-target.example"
yteam-control
```

Telegram uses the official Bot API long-polling endpoint. Discord and WhatsApp
use provider webhooks or a separate bridge that signs the canonical request:
`METHOD + "\n" + PATH + "\n" + BODY` with HMAC-SHA256 and sends it as
`X-YTEAM-Signature: sha256=<hex>`. YTEAM does not store bot tokens and does not
implement either provider's network protocol as a fake in-process client.

Only allowlisted actors can issue commands. `/bb` additionally requires an
exact target in `YTEAM_REMOTE_TARGET_ALLOWLIST`; `/quit` is permanently blocked
remotely. Every accepted or denied command is recorded in the runtime event
ledger. Never send cookies, API keys, passwords, or customer data through a
chat adapter.

## Architecture

```text
scripts/yteam_tui.py       terminal UI
scripts/yteam_runtime.py   commands, policy, events, model/session lifecycle
scripts/yteam_ai.py        direct OpenAI-compatible SSE client
scripts/yteam_state.py     SQLite WAL sessions, messages, and event sequences
scripts/yteam_session.py   session facade over durable state
scripts/yteam_models.py    local config + live model catalog
scripts/yteam_memory.py    verified two-phase learning memory
scripts/yteam_control.py   signed Telegram/webhook control plane
scripts/yteam_native_tools.py
                            smart pipe, secrets, knowledge, reports
scripts/yteam_hunt.py      scoped web recon and hypothesis handoff
scripts/yteam_engine.py    prerequisite-aware DAG orchestration
src/core/                  multi-pillar assessment control plane
skills/                    first-party native playbooks
```

Optional Camoufox is an isolated observer for browser classification and visual
review only. It does not solve challenges, evade WAFs, rotate identities, or
perform credential attacks.

## Local output

Runtime and engagement data is intentionally ignored by Git:

```text
runtime/bb-runs/<run-id>/
runtime/assessments/<run-id>/
runtime/sessions/<session-id>.jsonl
runtime/events.jsonl
```

Reports, evidence, recon dumps, secrets, packs, local model config, and browser
caches must stay local. Never commit cookies, bearer tokens, customer data, or
unredacted evidence.

## Verify

```powershell
python scripts\yteam_doctor.py --json
python -m compileall -q scripts src
python -m unittest discover -s tests -p "test_*.py" -v
git diff --check
```

## Authorized use

Use YTEAM only against assets explicitly authorized by a HackerOne, Bugcrowd,
Intigriti, or equivalent engagement. The default policy forbids destructive
actions, DoS, credential stuffing, customer-object access, resource claims, and
automatic report submission. See `REQUIREMENTS.md`, `SECURITY.md`, and
`docs/PUBLISHING.md`.
