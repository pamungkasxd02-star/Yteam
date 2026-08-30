# Yteam Security Workbench

Yteam is a source-based security workbench that combines:

- the upstream [OpenCode](https://github.com/anomalyco/opencode) terminal UI;
- the upstream [Hermes Agent](https://github.com/NousResearch/hermes-agent) agent core;
- the security, bug-bounty, QA, recon, triage, evidence, and reporting skills already maintained in the Hermes workspace.
- the upstream [Cybermes](https://github.com/Zyrexnn/Cybermes) tooling and knowledge adapters, extended by a Yteam emerging-bug intelligence layer.

This repository is **not affiliated with the OpenCode or Nous Research teams**. The upstream repositories are reproducibly fetched as separate checkouts under `vendor/` by `scripts/bootstrap_sources.py`; the public Yteam repository does not rewrite, embed, or vendor-copy their source files.

> **Status:** Yteam is an actively developed integration workbench. The Python
> control plane and Cybermes Go utilities are tested in CI. The upstream TUI
> requires Bun and its dependencies; CI checks those separately after the
> reproducible source bootstrap.

## Architecture

```text
OpenCode TUI (upstream TypeScript/Bun)
        |
        | OpenAI-compatible HTTP + SSE
        v
Hermes API server (upstream Python)
        |
        +-- Hermes AIAgent, memory, skills, delegation, browser, vision fallback
        +-- scoped terminal/file/web tools
        +-- direct Cybermes tools and knowledge adapters
        +-- evidence and report workflow
        +-- observation ledger → novelty hypotheses → validated candidates
```

## How to use YTEAM

YTEAM has one normal user flow. The `yteam` launcher is the entrypoint; the
Python scripts underneath it are orchestration internals and do not need to be
run manually for normal work.

```text
1. yteam
2. The launcher starts a temporary Hermes API bridge on 127.0.0.1.
3. The upstream OpenCode TUI connects to that bridge using the YTEAM model.
4. Chat commands are handled by the yteam-security agent.
5. /bb <authorized-target> starts the scope-safe assessment pipeline.
```

The request path is:

```text
OpenCode TUI
  → local OpenAI-compatible HTTP/SSE bridge (loopback only)
  → Hermes Agent model/tools/memory/browser/vision routing
  → YTEAM control plane and Cybermes local utilities
  → target-scoped runtime artifacts, hypotheses, evidence, and reports
```

The visible model name in the TUI is **`YTEAM`**. The technical provider/model
identifier remains `yteam/yteam-agent`; this is intentional and should not be
changed when selecting the model in configuration. The visible agent is
`yteam-security` and the visible TUI footer is `YTEAM`.

## Botterdop: bot/WAF detection and safe handling

**Botterdop** is Yteam's anti-automation detection subsystem. It is a
classifier and request governor, not a WAF-bypass tool. It inspects response
headers, status codes, and bounded response text to identify common gates:

| Detection | Typical signals | Safe action |
|---|---|---|
| Cloudflare challenge/managed | `cf-ray` with challenge body, `cf-mitigated`, `managed_check` | Stop active probing |
| Akamai KPSDK | `window.KPSDK`, `akamai-bm`, `_abck` with a block response | Stop active probing |
| DataDome | `datadome`, `dd_cookie_test`, captcha response | Stop and request manual review |
| Kasada | `kasada`, `x-kpsdk-cid`, `x-kpsdk-ct` | Stop active probing |
| HUMAN/PerimeterX | `perimeterx`, `px-captcha`, `_px3` with a block response | Stop active probing |
| reCAPTCHA/Enterprise | `grecaptcha.render`, Enterprise API markers | Stop and request manual review |
| Turnstile | `cf-turnstile`, Turnstile API markers | Stop and request manual review |
| HTTP rate limit | status `429`, optional `Retry-After` | Back off and slow down |
| Generic WAF/custom block | explicit `403` block wording or provider marker | Stop active probing |

The detector records `gate`, `category`, `confidence`, matched `evidence`, and
an action of `continue`, `slow_down`, `manual_review`, or `stop`. When the
action is `stop` or `manual_review`, the native recon engine halts subsequent
document, crawl, and passive requests. A normal CDN correlation header on a
successful response is not treated as a gate by itself.

Botterdop does **not** solve CAPTCHA, rotate identities, evade fingerprints,
spray headers, bypass WAF rules, or run credential attacks. For an authorized
assessment, the correct next step is to preserve the blocker evidence and use
an approved human/browser workflow or contact the program—not to evade the
control.

The Botterdop result is available in:

```text
runtime/.../recon.json                 # per-response observations + summary
runtime/.../pillars/bot_bypass.json   # unified assessment output
```

## Botterdop + Camoufox

Botterdop can use **Camoufox** as an optional isolated browser observer. It
captures bounded same-origin response metadata, sends status/headers/body
signals to Botterdop, and records `continue`, `slow_down`, `manual_review`, or
`stop`. A detected WAF, bot gate, or CAPTCHA stops further observation; it is
not treated as a bypass.

Use it through the normal autonomous flow or directly:

```text
/bb https://authorized-target.example
```

```powershell
python scripts\botterdop.py https://authorized-target.example
python scripts\botterdop.py https://authorized-target.example --headed
python scripts\botterdop.py https://authorized-target.example --scope-file scope.yaml
```

Install Camoufox only in the active Yteam/Hermes environment:

```powershell
python -m pip install camoufox
python -m camoufox fetch
```

Keep its browser cache on the data drive and never use a personal browser
profile. Camoufox mode is detection/manual-review only: no CAPTCHA solving,
fingerprint evasion, identity rotation, proxy changes, credential attacks,
scraping-at-scale, or WAF bypass is performed.

### First run

From the repository root:

```powershell
python .\scripts\bootstrap_sources.py
Set-Location vendor\hermes-agent
uv venv --python 3.11 .venv
uv pip install -e ".[all]"
Set-Location ..\opencode
bun install
Set-Location ..\..
python .\scripts\install_yteam.py
```

Put the model, provider, API key, and optional base URL in one local file:
`yteam.local.yaml`. Copy `yteam.local.example.yaml`, fill in `api_key`, and
keep the file in the project root. It is ignored by Git. The launcher persists
only non-secret routing (`model.provider`, `model.default`, and `model.base_url`)
to the active Hermes `config.yaml`; the API key is passed only to the child
process at runtime. The first launch creates the active profile under
`runtime/yteam-hermes-home/`; for a separate durable profile, set
`YTEAM_HERMES_HOME` to that profile directory.

Quick setup:

```powershell
Copy-Item .\yteam.local.example.yaml .\yteam.local.yaml
notepad .\yteam.local.yaml
```

Then fill in the four fields at the top:

```yaml
provider: openrouter
model: anthropic/claude-sonnet-4
api_key: "your-key-here"
base_url: "https://openrouter.ai/api/v1"
```

Run `yteam` afterward. No separate `.env` file is required for the main model
when this convenience file is used.

Then open a new terminal and run:

```text
yteam
```

### Custom chat command

Inside the TUI:

| Command | Purpose |
|---|---|
| `/bb <target>` | Full autonomous, scope-gated bug-bounty/recon run |
| Other slash commands | Native OpenCode commands |

YTEAM intentionally registers only one custom slash command. Internal helpers
remain available to the `/bb` workflow and can be run from the terminal for
diagnostics; they are not extra TUI commands.

For the YTEAM autonomous path, use:

```text
/bb https://authorized-target.example
```

YTEAM reads scope/locks/prior work first, maps the target with low-rate
read-only requests, selects matching Cybermes skills, creates hypotheses, and
stops at the evidence/triage boundary. It does not automatically submit a
report. A status of `PACK`, `CAND`, `MID`, `BLOCKED`, or `0` is recorded only
when supported by the available evidence.

### Where results go

Each run is isolated under the D: workspace:

```text
runtime/bb-runs/<run-id>/<target>/
runtime/assessments/<run-id>/<target>/
```

The important files are `scope.json`, `recon/recon.json`, `track_plan.json`,
`hypotheses.json`, `hunt_context.md`, `assessment_manifest.json`, and the
target-scoped evidence/report directories. Raw output stays on disk rather
than being dumped into the chat. Cookies, bearer tokens, passwords, API keys,
and unrelated PII are redacted before durable evidence is written.

### Technical commands (optional)

These are useful for diagnostics, CI, and development, but are not required for
normal TUI use:

```powershell
python .\scripts\yteam_toolchain.py --json
python .\scripts\yteam_engine.py --list-engines
python .\scripts\yteam_doctor.py --json
python .\scripts\yteam_hidden.py <recon.json> --output <target-dir>\hidden_surface.json
python .\scripts\yteam_assessment.py https://authorized-target.example
python -m unittest discover -s tests -p "test_*.py" -v
```

### One unified assessment run

Yteam is a single platform, not five unrelated scripts. `src/core/platform.py` owns the shared `AssessmentContext`, fail-closed `Policy`, thread-safe `EventBus`, target-scoped `ArtifactStore`, and `EngineRegistry`. `src/core/assessment.py` registers the five pillars plus recon, intelligence, learning, and delivery in one dependency graph:

```text
scope → toolchain → recon → { bot-bypass, decrypt, pentest/QA, server-guard }
                                      ↓
                                intelligence
                                      ↓
                                  learning
                                      ↓
                                  delivery
```

The user-facing entrypoint is still only:

```text
yteam
/bb <authorized-target>
```

The run automatically writes `assessment_manifest.json`, `assessment_context.md`, and `assessment_context.json` under `runtime/assessments/<run-id>/<target-slug>/`. The LLM receives a compact context contract and uses the relevant pillar artifacts instead of guessing from raw tool output.

The OpenCode TUI is the presentation layer. Yteam/Hermes is the execution and learning layer. This is deliberate: OpenCode's native server protocol is not the same protocol as Hermes' gateway API, while Hermes already exposes a documented OpenAI-compatible API server. The bridge therefore avoids a fragile protocol fork and lets both upstream projects update independently.

### Multi-engine orchestration

Beyond the linear Cybermes-style pipeline, Yteam runs a **DAG-driven multi-engine orchestrator** (`scripts/yteam_engine.py`) that schedules sub-engines adaptively based on run state and prerequisites:

```text
scope
  └→ inventory
        ├→ passive
        └→ recon
              └→ fingerprint
                    └→ mapping
                          └→ intel
                                └→ validation
                                      └→ triage
                                            └→ delivery
```

Each phase only runs once its dependency phase completes. Engines are modular (`scope`, `inventory`, `passive`, `recon`, `fingerprint`, `mapping`, `intel`, `validation`, `triage`, `delivery`) and registered through `make_registry()`.

Supporting layers:

- **`scripts/yteam_parallel.py`** — rate-limited parallel read-only recon scheduler.
- **`scripts/yteam_knowledge.py`** — durable cross-run knowledge base (hypothesis verdicts, signatures, dedupe) so later runs skip ground already tested.
- **`scripts/yteam_intelligence.py`** — multi-dimensional emerging-bug engine (actor/scope/state/behavior differentials, unknown-class detection, track routing).
- **`scripts/yteam_skills.py`** — complete 315-skill registry + adaptive bundle selection.
- **`scripts/yteam_hunt.py`** — target-scoped hunt manifest, toolchain, track plan, skill bundle, hypotheses, and LLM hunt context.
- **`scripts/yteam_hidden.py`** — bug-first route graph and trust-boundary planner for hidden/sibling/version/API behavior.

### Hidden-bug discovery planner

After native recon, YTEAM builds `hidden_surface.json`. This layer helps find
bugs that ordinary URL crawling misses. It correlates:

- object references in paths and query parameters (`id`, `uid`, `tenant`,
  `order`, `invoice`, `document`, and related identifiers);
- sibling endpoint families and inconsistent response/auth coverage;
- versioned API routes that may have authorization drift;
- GraphQL routes that overlap with REST resources;
- URL-processing flows such as preview, proxy, callback, webhook, and render;
- authentication/reset/OAuth route families;
- business-state families such as order, invoice, refund, invite, and export;
- parameters, path IDs, sources, status, content type, and route priority.

The planner produces ranked **hypotheses** and bounded `safe_checks`. Each
check states its prerequisite, success signal, and stop signal. Hermes selects
one high-value hypothesis at a time and validates it through the normal scope,
Botterdop, researcher-owned fixture, evidence, and triage gates.

It does not enumerate customer IDs, access foreign objects, generate exploit
payloads, perform destructive writes, or turn a route difference into a
finding. A route containing `id` is only a candidate until a cross-identity
security-boundary violation is demonstrated.

Artifact locations:

```text
runtime/.../hidden_surface.json
runtime/.../hunt_context.md
```

Drive the orchestrator with:

```bash
python scripts/yteam_run.py --engine <target>          # DAG multi-engine run
python scripts/yteam_run.py <target>                   # linear hunt driver
python scripts/yteam_engine.py --list-engines          # show engine + DAG
```


## Vision fallback

The main model does not need native vision. OpenCode sends an image attachment through the OpenAI-compatible request; Hermes applies its normal image-routing policy:

1. use native vision when the selected model supports it;
2. otherwise route the image to Hermes' configured auxiliary `vision` model / `vision_analyze` tool;
3. if no vision backend is configured, return a clear text-only limitation instead of pretending to have inspected pixels.

Configure the auxiliary vision model in the active Hermes `config.yaml`, for example:

```yaml
auxiliary:
  vision:
    provider: openrouter
    model: google/gemini-2.5-flash
agent:
  image_input_mode: auto
```

The security profile also supports text-first workflows: HAR files, HTTP transcripts, screenshots converted to OCR/text, source bundles, JSON, and evidence notes can be analyzed even when no vision provider is available.

## Source checkouts

The initial checkout is kept under:

```text
vendor/opencode/       # anomalyco/opencode, branch dev
vendor/hermes-agent/   # NousResearch/hermes-agent, branch main
```

Update them explicitly from their upstream remotes; do not update the installed copy in `%LOCALAPPDATA%` or `~/.hermes` as a substitute for source development.

## Requirements

### Minimum runtime

| Requirement | Minimum | Recommended | Used by |
|---|---:|---:|---|
| Operating system | Windows 10/11, macOS 12+, or modern Linux | Windows 11, macOS 14+, Ubuntu 22.04+ | All layers |
| Git | 2.40+ | Latest stable | Source bootstrap and updates |
| Python | 3.11–3.13 | Python 3.11 or 3.12 | Hermes, Yteam engines, QA modules |
| Bun | 1.1+ | Latest stable | Upstream OpenCode TUI |
| Go | 1.22+ | Go 1.25+ | Cybermes native utilities/source fallback |
| RAM | 4 GB | 8–16 GB | TUI, Hermes, optional scanners |
| Disk | 5 GB free | 15 GB+ on D:/data volume | Dependencies, source, evidence, caches |
| Network | HTTPS access to configured model provider | Stable broadband | Model calls, passive sources, authorized targets |

Python, Git, and Bun are required for the normal TUI flow. Go is optional but recommended: without Go, the Cybermes direct utilities are marked unavailable while the native Python recon/analysis workflow still works. External tools such as Katana, ProjectDiscovery HTTPX, Subfinder, `gau`, `waybackurls`, Nuclei, FFUF, Arjun, Nmap, Naabu, Dalfox, and SQLMap are optional; Yteam detects them and uses missing-safe fallbacks.

Quick setup requirements: `REQUIREMENTS.md`. Full dependency, tool, OS, model,
browser, and feature matrix: `docs/REQUIREMENTS.md`.

The TUI's local OpenCode tool permissions are intentionally disabled in this integration. This prevents a second, competing tool loop: Hermes remains the single owner of terminal, browser, memory, skills, vision, evidence, and report execution.

The bundled `.opencode/plugins/yteam-tui.tsx` replaces the home branding with Yteam without modifying the upstream OpenCode TUI source.

The project never stores API keys in `opencode.json`. The launcher creates a
short-lived loopback bridge key at runtime and passes it only to the child
processes. The first launch creates a separate YTEAM profile in
`runtime/yteam-hermes-home/` with `SOUL.md`, `MEMORY.md`, `USER.md`, and
`config.yaml`. The convenience file `yteam.local.yaml` is the single place for
the provider credential; the launcher maps it to the child process without
persisting the API key in the Hermes config.

Learning is deliberate rather than raw transcript scraping: Hermes' background review proposes compact, verified lessons after turns, and the `memory` tool persists approved non-secret facts. The system prompt uses a session-start snapshot to preserve prompt caching, so saved lessons become authoritative on the next session while the current conversation remains available immediately.

## Start

### Recommended: `yteam`

Install the tiny user-local launcher once:

```powershell
python .\scripts\install_yteam.py
```

Add the printed bin directory to your user `PATH`, open a new terminal, then from any directory run:

```text
yteam
```

That opens the Yteam-branded OpenCode TUI as a normal chat interface. No Python command is needed afterward. The existing global `opencode` installation is not modified.

### Windows PowerShell

```powershell
python .\scripts\hermes_opencode.py
# or
.\scripts\hermes_opencode.cmd
# or, when the project directory is on PATH
yteam
```

### macOS / Linux

```bash
python3 scripts/hermes_opencode.py
```

The repository includes a POSIX launcher at `./yteam`; make it executable with `chmod +x yteam` if needed.

OpenCode arguments are forwarded by the launcher:

```bash
python3 scripts/hermes_opencode.py --continue
python3 scripts/hermes_opencode.py --session <session-id>
```

The launcher starts Hermes' loopback API server, waits for `/health`, then starts OpenCode's upstream TUI against this project directory. Press `Ctrl+C` to stop the TUI and clean up the child gateway.

Native OpenCode commands are preserved. If the first argument is an upstream command such as `serve`, `acp`, `attach`, `mcp`, `models`, `run`, `debug`, `session`, `providers`, or `web`, the launcher skips the Yteam bridge bootstrap and executes the upstream command directly with the project configuration. The `mcp` entry here is only the original OpenCode command; Yteam does not configure a Cybermes MCP server.

## First-time source setup

From the project root:

```bash
cd vendor/hermes-agent
uv venv --python 3.11 .venv
uv pip install -e ".[all]"

cd ../opencode
bun install
```

The launcher honors `HERMES_PYTHON` when a different Python executable is needed. It honors `BUN_BIN` when Bun is not on `PATH`. The user-facing binary/launcher name is `yteam`.

Cybermes direct utilities are available through `scripts/cybermes.py` (`smart-pipe`, `secret-scan`, `search-knowledge`, and `aggregate-reports`). If Go is available, the selected utility runs with `go run`; for faster startup, build the utility into `runtime/bin/` and the wrapper uses that binary instead. No MCP server is started by Yteam. `/bb` is the autonomous orchestrator; these utilities are supporting primitives selected by the Hermes runtime, not a blind scanner.

To use another durable Yteam profile:

```powershell
$env:YTEAM_HERMES_HOME = "D:\\Yteam\\profile"
python .\scripts\hermes_opencode.py
```

The initializer never overwrites an existing SOUL, memory, config, session, or log file.

For publishing this project as a clean GitHub repository, follow `docs/PUBLISHING.md`. Nested upstream checkouts are intentionally ignored by the root `.gitignore` and are restored by the bootstrap script after cloning.

## Skill sources

`opencode.json` registers these directories as skill sources:

- `.opencode/skills/` — integration-specific skills;
- `../.agents/skills/` — the workspace's security playbooks;
- `../.opencode/skills/` — workspace security bridge skills;
- `vendor/hermes-agent/skills/` — upstream Hermes bundled skills;
- `vendor/hermes-agent/optional-skills/security/` — upstream optional security skills.
- `vendor/cybermes/skills/` — upstream Cybermes security skills.

Use the catalog helper to inspect the complete combined registry without loading every skill into the model context:

```bash
python3 scripts/index_skills.py --output runtime/skill-catalog.json
```

The deeper registry records every discovered upstream Cybermes skill with its source, category, description, and SHA-256 content hash:

```bash
python3 scripts/yteam_skills.py index --output runtime/cybermes-skill-registry.json
python3 scripts/yteam_skills.py bundle --signals api graphql authorization --limit 24
```

`/bb` runs both automatically and writes the selected bundle beside `hunt_context.md`. The full catalog remains available on disk; only target-relevant SOPs are promoted into the model's active hunt context.

Emerging-bug intelligence commands:

```bash
python3 scripts/yteam_intelligence.py record --input '{"target":"local","endpoint":"/api/test","method":"GET","status":200,"response_length":42,"actor":"anonymous","scope":"tenant-a","tags":[],"source":"qa-wave-1"}'
python3 scripts/yteam_intelligence.py analyze
```

On PowerShell, use stdin or a JSON file so shell quoting cannot corrupt the observation:

```powershell
'{"target":"local","endpoint":"/api/test","method":"GET","status":200,"response_length":42,"actor":"anonymous","scope":"tenant-a","tags":[],"source":"qa-wave-1"}' | python .\scripts\yteam_intelligence.py record --input -
```

The analyzer is intentionally conservative. It detects unusual combinations and response differentials, but emits only `emerging_bug_hypothesis` records with provenance, novelty, confidence, and a next safe test. It never writes into `findings/` and never replaces the normal triage gate.

The one-command flow is:

```text
/bb <program, exact target URL, or existing pack path>
```

It follows Cybermes' scope → recon → map → adaptive track plan → hypothesis → validate → triage → delivery pipeline and writes a durable run record under `runtime/bb-runs/`. Each run has separate `recon/`, `evidence/`, `reports/`, and `intelligence/` paths so targets cannot contaminate one another.

## Security operating contract

This workbench is for authorized penetration testing, bug bounty, QA, and defensive validation only. The profile requires scope-first work, read-only probes by default, low request rates, researcher-owned fixtures, no credential stuffing, no destructive changes, and evidence-backed findings. Pure metadata, public catalogs, SPA shells, ordinary auth errors, and unproven theoretical impact are recorded as notes—not reported as vulnerabilities.

Reports and scripts are English. Operator chat may remain Indonesian.

## Upstream attribution and licenses

OpenCode and Hermes Agent are MIT-licensed upstream projects. Preserve their licenses and attribution when distributing this integration. See `vendor/opencode/LICENSE` and `vendor/hermes-agent/LICENSE`.
