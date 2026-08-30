# YTEAM Requirements

Read this file before running the installer.

## Required

| Software | Version | Purpose |
|---|---:|---|
| Git | 2.40+ | Clone and update YTEAM |
| Python | 3.11–3.13 | Run the native YTEAM runtime |
| PowerShell or POSIX shell | Current | Run the installer |
| Internet | HTTPS | Install dependencies and call the model provider |
| Model API key | Not required for Zen Free; provider-specific for overrides | Run the AI client |

The native YTEAM TUI does not require Bun, Go, an external agent runtime, or a
separate UI application.

## Supported systems

- Windows 10/11 x64;
- macOS 12+ Intel/Apple Silicon;
- Linux x64/arm64;
- WSL2 using the Linux instructions.

## Installed automatically

`python scripts/install_yteam.py` creates a local runtime tree:

- Native YTEAM TUI under `scripts/yteam_tui.py`;
- Native policy, session, event, model, skill, and assessment components;
- YTEAM virtual environment at `runtime/.venv`;
- PyYAML and optional Camoufox dependencies from `requirements.txt`;
- the user-local `yteam` launcher.

No vendor checkout is downloaded. No global OpenCode, Bun, or shell tool is
modified. Generated runtime and engagement artifacts remain local.

## Optional components

| Component | Purpose | If missing |
|---|---|---|
| Camoufox | Isolated browser observation for anti-bot classification | Native HTTP detection |
| Chrome/Chromium/Edge | Browser proof and visual QA | HTTP/text workflows |
| Nuclei, FFUF, Arjun, Dalfox, SQLMap | Targeted validation helpers | Manual safe validation |
| Go-based helpers | Optional external tooling | Native/Python fallbacks |

Camoufox is installed from the single root requirements file. Skip only its
browser binary download with:

```text
python scripts/install_yteam.py --skip-browser-download
```

## Model

YTEAM automatically discovers the current Zen Free catalog and starts with:

```text
provider: zen-free
model: laguna-s-2.1-free
base_url: https://opencode.ai/zen/v1
```

The native `/models` command displays the live choices. If the catalog is
temporarily unavailable, a bundled fallback list is used. No API key is needed
for the free default.

To override it, copy `yteam.local.example.yaml` to `yteam.local.yaml`. That
file is ignored by Git and the key is held only in memory by the native client.

## Disk and local data

Keep dependencies, browser downloads, runtime data, evidence, and reports on a
data drive when possible. Ignored local data includes:

```text
yteam.local.yaml
runtime/
reports/
evidence/
recon/
secrets/
packs/
```

## Verify

```powershell
python scripts\yteam_doctor.py --json
python -m compileall -q scripts src
python -m unittest discover -s tests -p "test_*.py" -v
```

## Safety

YTEAM is for authorized security testing only. The default mode is scoped,
read-only, low-rate, and non-destructive. Botterdop/Camoufox detects gates but
does not bypass WAFs or solve challenges. `/bb` never submits reports
automatically.
