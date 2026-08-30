# YTEAM Requirements

This is the quick, root-level requirements guide shown directly on GitHub.
For the longer platform matrix and implementation notes, see

## Required before running `yteam`

| Requirement | Minimum | Why it is needed |
|---|---:|---|
| Git | 2.40+ | Fetch OpenCode, Hermes Agent, and Cybermes sources |
| Python | 3.11–3.13 | YTEAM orchestration and Hermes runtime |
| uv | Current stable | Create/install the Hermes Python environment |
| Bun | 1.1+ | Run the upstream OpenCode TUI and install its dependencies |
| Model provider | One configured provider | Generate responses and perform model reasoning |
| Network | HTTPS access | Fetch sources, call the configured model, and assess only authorized targets |

Supported platforms:

- Windows 10/11 x64;
- macOS 12+ Intel/Apple Silicon;
- Linux x64/arm64;
- WSL2 as a Linux-style installation.

## Optional components

| Component | Use | If absent |
|---|---|---|
| Go 1.22+ | Fast Cybermes native utilities | Python/native fallbacks remain available |
| Camoufox | Isolated Botterdop browser observation | Native HTTP Botterdop detection remains available |
| Chrome/Chromium/Edge | Browser proof and visual QA | Text/HTTP-only workflows remain available |
| Katana, Subfinder, ProjectDiscovery HTTPX | Additional recon | Native bounded recon remains available |
| Nuclei, FFUF, Arjun, Dalfox, SQLMap | Targeted validation helpers | Hypothesis-driven native/manual validation |

## Upstream source setup

The public repository intentionally does not commit the upstream source trees.
After cloning, run:

```powershell
python scripts\bootstrap_sources.py
```

This fetches:

```text
vendor/opencode/       # OpenCode, branch dev
vendor/hermes-agent/   # Hermes Agent, branch main
vendor/cybermes/      # Cybermes, branch main; sparse checkout
```

Install dependencies:

```powershell
Set-Location vendor\hermes-agent
uv venv --python 3.11 .venv
uv pip install -e ".[all]"
Set-Location ..\opencode
bun install
Set-Location ..\..
```

## Model configuration

YTEAM uses one convenient local file at the repository root:

```text
yteam.local.yaml
```

Create it from the safe template:

```powershell
Copy-Item .\yteam.local.example.yaml .\yteam.local.yaml
notepad .\yteam.local.yaml
```

Minimum example:

```yaml
provider: openrouter
model: anthropic/claude-sonnet-4
api_key: "your-provider-key"
base_url: "https://openrouter.ai/api/v1"
```

`yteam.local.yaml` is ignored by Git. The API key is passed to the Hermes child
process at runtime and is not persisted in the profile routing config.

## Camoufox/Botterdop requirements

Camoufox is optional. Install it only in the active YTEAM/Hermes environment:

```powershell
python -m pip install camoufox
python -m camoufox fetch
```

Botterdop uses Camoufox only for bounded, isolated, authorized browser
observation. It detects bot/WAF/CAPTCHA gates and chooses `continue`,
`slow_down`, `manual_review`, or `stop`. It does not solve CAPTCHA, evade WAFs,
rotate identities, rotate proxies, credential-stuff, scrape at scale, or run
destructive actions.

## Disk and privacy

Keep source caches, browser downloads, runtime artifacts, sessions, logs,
screenshots, HAR files, evidence, reports, and temporary files on a data drive.
Recommended free space is 5 GB minimum and 15 GB or more for a complete local
setup. Never commit:

```text
yteam.local.yaml
runtime/
reports/
evidence/
recon/
secrets/
packs/
auth.json
auth.lock
```

## Verification

Run the local doctor and tests:

```powershell
python scripts\yteam_doctor.py --json
python -m compileall -q scripts src
python -m unittest discover -s tests -p "test_*.py" -v
```

The TUI entrypoint is:

```powershell
python scripts\install_yteam.py
```

Inside the TUI, YTEAM adds only one custom command:

```text
/bb https://authorized-target.example
```

All other slash commands remain native OpenCode commands. Testing is allowed
only against systems owned by the operator, explicitly authorized systems, or
in-scope bug-bounty/pentest assets.
