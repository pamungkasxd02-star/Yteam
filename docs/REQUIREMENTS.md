# Yteam Platform Requirements

This document is the complete requirements reference for a clean GitHub clone.
Yteam is a source-based integration: the OpenCode TUI, Hermes Agent runtime,
and Cybermes source dependencies are fetched explicitly by the bootstrap
script rather than silently downloaded at runtime.

## Supported operating systems

| OS | Status | Notes |
|---|---|---|
| Windows 10/11 x64 | Supported | Use PowerShell or Command Prompt. Long-path support is configured for the Cybermes sparse checkout. |
| macOS 12+ Intel/Apple Silicon | Supported | Use Terminal, Homebrew, or the official Bun installer. |
| Linux x64/arm64 | Supported | Ubuntu/Debian/Arch/Fedora are expected to work; use the distribution's Python 3.11+ and Bun. |
| WSL2 | Supported | Treat the WSL checkout as a Linux checkout; use Windows Chrome/CDP only when explicitly configured. |
| Docker/Podman | Planned/optional | The upstream Hermes and Cybermes sources contain container assets, but the Yteam unified wrapper is currently validated natively first. |

## Required software

### Git

Git is required to fetch the upstream sources:

```text
git --version
```

Recommended: Git 2.40 or newer. The bootstrap uses shallow and sparse clone
features supported by current Git versions.

### Python

Hermes currently requires Python `>=3.11,<3.14`.

```text
python --version       # Windows
python3 --version      # macOS/Linux
```

The following are installed into the Hermes virtual environment by the
upstream project:

```text
uv
openai
httpx[socks]
pydantic
prompt_toolkit
PyYAML
Jinja2
Pillow
FastAPI/Uvicorn
websockets
PyJWT[crypto]
```

Do not install these into the system Python unless you intentionally manage
that environment. Use the upstream Hermes `uv` flow instead.

### Bun

Bun runs the upstream OpenCode TypeScript TUI and its workspace dependencies.

```text
bun --version
```

The source checkout expects Bun as its package manager. `npm` may work for
some package operations, but Bun is the supported path for this repository.

### Go (recommended, optional)

Go is required for fast Cybermes direct utilities and optional for native
Python fallback paths:

```text
go version
```

Yteam uses Go module/cache paths on the data drive when configured. It does not
need root/admin access to compile the Cybermes utilities.

## Install from GitHub

From a fresh clone:

```text
git clone https://github.com/<account>/<yteam-repository>.git Yteam
cd Yteam
python scripts/bootstrap_sources.py
```

Windows:

```powershell
git clone https://github.com/<account>/<yteam-repository>.git Yteam
Set-Location Yteam
python .\scripts\bootstrap_sources.py
```

Install Hermes:

```powershell
Set-Location vendor\hermes-agent
uv venv --python 3.11 .venv
uv pip install -e ".[all]"
```

macOS/Linux:

```bash
cd vendor/hermes-agent
uv venv --python 3.11 .venv
uv pip install -e ".[all]"
```

Install OpenCode source dependencies:

```text
cd vendor/opencode
bun install
```

Return to the Yteam root and install the user-facing launcher:

```text
python scripts/install_yteam.py       # Windows
python3 scripts/install_yteam.py      # macOS/Linux
```

Open a new terminal after adding the printed bin directory to `PATH`.

## Model provider requirements

Yteam does not ship or commit model credentials. Configure one provider in the
local root file `yteam.local.yaml`, copied from `yteam.local.example.yaml`.
That single file contains the provider, model, API key, and optional base URL.
The launcher persists only non-secret routing in the Hermes profile and passes
the API key to Hermes ephemerally.

Common options:

| Provider type | Credential/config requirement | Vision |
|---|---|---|
| OpenRouter | `yteam.local.yaml` with provider/model/API key | Configure an auxiliary vision model if main model is text-only |
| OpenAI-compatible cloud | `yteam.local.yaml` with API key + base URL/model | Depends on selected model |
| Anthropic/Codex/provider OAuth | Upstream Hermes setup flow | Depends on selected model |
| Local Ollama/LM Studio/vLLM | Local endpoint in Hermes config | Usually text-only unless model supports images |
| Nous Portal | Upstream Hermes portal setup | Provider-dependent; auxiliary vision can be selected |

For a text-only main model, configure the optional `vision` block in
`yteam.local.yaml`, then mirror it into the active Hermes profile's auxiliary
section:

```yaml
agent:
  image_input_mode: auto
auxiliary:
  vision:
    provider: openrouter
    model: google/gemini-2.5-flash
```

Yteam routes images through native vision, auxiliary Hermes vision, or text/OCR
fallback. It never claims visual inspection when no vision path exists.

## Browser requirements

Browser automation is optional for pure recon. It is needed for browser proof
of CORS/XSS/CSRF and for visual QA.

Recommended:

- Chromium/Chrome/Edge for local browser control;
- Playwright/Puppeteer support through the upstream Hermes browser setup;
- a separate research browser profile, never the user's personal profile;
- no proxy reconfiguration by Yteam; provide an already authorized network path
  only when the engagement permits it.

Optional Botterdop browser mode uses Camoufox in a fresh isolated context. It
is an observation/manual-review adapter only; it does not solve CAPTCHA or
evade WAFs. Keep Camoufox's browser cache on the data drive and never point it
at a personal browser profile. `/bb` enables the adapter; if Camoufox is not
installed, Yteam records `unavailable` and keeps native HTTP detection active.

Install only when browser observation is required, inside the active Yteam
environment:

```text
python -m pip install camoufox
python -m camoufox fetch
```

## Optional security tools

Yteam detects these tools without requiring all of them:

| Tool | Capability | If missing |
|---|---|---|
| Camoufox | Isolated Botterdop browser observation | Native HTTP Botterdop detection and manual review status |
| ProjectDiscovery `httpx` | HTTP probing/fingerprinting | Native Python/Go baseline probe |
| Katana | Deep crawler/JS mining | Native bounded crawler |
| Subfinder | Passive subdomains | CT/passive fallback or no active asset expansion |
| `gau`, `waybackurls` | Archive URL collection | No archive stage |
| Nuclei | Limited CVE/misconfig verification | Manual Hermes hypothesis validation |
| FFUF/Feroxbuster/Arjun | Content/parameter candidates | Targeted route checks only |
| Nmap/Naabu | Service/port inventory | HTTP-only assessment |
| Dalfox | XSS candidate analysis | Manual browser proof |
| SQLMap | SQLi verification | Manual safe SQLi checks |
| Cybermes Go utilities | Smart filtering, knowledge, secrets, report aggregation | Python/native fallbacks where available |

The current inventory can be viewed with:

```text
python scripts/yteam_toolchain.py --json
```

Missing optional tools do not automatically mean setup failed.

## Resource requirements

Recommended free space:

```text
Source checkouts:       2–5 GB
Hermes Python venv:      1–3 GB
OpenCode node_modules:   1–4 GB
Cybermes cache/KB:       1–10+ GB depending on sources
Assessment evidence:     variable; keep on a data drive
```

Keep generated runtime, caches, session data, logs, HARs, screenshots, videos,
and evidence on a data volume. The repository's `.gitignore` excludes local
runtime and credentials from GitHub commits.

## Verification

After setup:

```text
yteam --version
python scripts/yteam_toolchain.py --json
python scripts/yteam_engine.py --list-engines
python scripts/yteam_skills.py index --output runtime/cybermes-skill-registry.json
python -m unittest discover -s tests -p "test_*.py" -v
```

For Cybermes Go packages:

```text
cd vendor/cybermes
```

## Feature availability matrix

| Feature | Python | Bun | Go | Model | Browser | External tools |
|---|---:|---:|---:|---:|---:|---:|
| TUI chat | required | required | no | required | no | no |
| `/bb` scope/recon | required | TUI only | optional | required for follow-up reasoning | no | optional |
| Native recon fallback | required | no | no | no | no | no |
| Cybermes direct utilities | required wrapper | no | recommended | no | no | no |
| Bot gate classification | required | no | no | no | no | no |
| Decrypt/format analysis | required | no | no | no | no | no |
| Pentest/QA matrix | required | no | no | no | no | no |
| Server guard report | required | no | no | no | no | no |
| Browser CORS/XSS/CSRF proof | required | TUI optional | no | required | required | no |
| Nuclei/CVE verification | required | no | no | required for triage | no | Nuclei |

### Botterdop behavior

Botterdop is included in the Python integration layer and requires no separate
service or API key. It classifies Cloudflare, Akamai KPSDK, DataDome, Kasada,
HUMAN/PerimeterX, reCAPTCHA, Turnstile, generic WAF blocks, and HTTP 429 rate
limits. It applies safe actions only: continue, slow down, manual review, or
stop. It never performs challenge solving or WAF evasion. A provider header
such as `CF-Ray` on an ordinary successful response is not sufficient by
itself to halt a run.

## Legal requirement

Technical dependencies are not authorization. Before `/bb` performs a live
assessment, the operator must have written permission or an in-scope bug bounty
asset. Use the scope file and target pack; do not test customer systems or
invent sibling assets.
