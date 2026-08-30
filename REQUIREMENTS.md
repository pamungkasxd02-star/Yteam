# YTEAM Requirements

Read this file before running the installer.

## Required

| Software | Version | Purpose |
|---|---:|---|
| Git | 2.40+ | Download upstream sources |
| Python | 3.11–3.13 | Run YTEAM and Hermes |
| PowerShell or POSIX shell | Current | Run the installer |
| Internet | HTTPS | Download sources/dependencies and call the model provider |
| Model API key | Provider-specific | Run the AI agent |

The TUI also requires Bun. The installer installs Bun automatically when it is
not already available.

## Supported systems

- Windows 10/11 x64;
- macOS 12+ Intel/Apple Silicon;
- Linux x64/arm64;
- WSL2 using the Linux instructions.

## Installed automatically

`python scripts/install_yteam.py` installs or prepares:

- OpenCode under `vendor/opencode`;
- Hermes Agent under `vendor/hermes-agent`;
- Cybermes under `vendor/cybermes`;
- Hermes Python environment at `vendor/hermes-agent/.venv`;
- OpenCode JavaScript dependencies with `bun install`;
- Camoufox and its browser runtime;
- the user-local `yteam` launcher.

Dependency files are kept at the repository root:

```text
requirements.txt          # all direct YTEAM Python dependencies, including Camoufox
```

The upstream source directories are not committed to this repository. The
bootstrap script fetches them using the revisions recorded in `vendor/SOURCES.md`.

## Optional components

| Component | Purpose | If missing |
|---|---|---|
| Go 1.22+ | Faster Cybermes utilities | Native/Python fallbacks |
| Camoufox | Browser observation for Botterdop | Native HTTP detection |
| Chrome/Chromium/Edge | Browser proof and visual QA | HTTP/text workflows |
| Nuclei, FFUF, Arjun, Dalfox, SQLMap | Targeted validation helpers | Manual safe validation |

Camoufox is installed by default from the single `requirements.txt`. Skip only
the browser binary download with:

```text
python scripts/install_yteam.py --skip-browser-download
```

## Model configuration

Create the local configuration file:

```text
yteam.local.yaml
```

from:

```text
yteam.local.example.yaml
```

Example:

```yaml
provider: openrouter
model: anthropic/claude-sonnet-4
api_key: "your-api-key"
base_url: "https://openrouter.ai/api/v1"
```

This file is ignored by Git. Never publish it. The API key is passed to the
Hermes child process at runtime and is not stored in the generated routing
config.

## Disk

Keep dependencies, browser downloads, runtime data, evidence, and reports on a
data drive when possible. Allow at least 5 GB free; 15 GB is recommended for
all upstream dependencies and browser data.

Ignored local data includes:

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
