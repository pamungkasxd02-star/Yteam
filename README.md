# YTEAM

YTEAM is a local security-research workbench built around the upstream
OpenCode terminal UI, Hermes Agent runtime, and Cybermes knowledge/tools.

It has one custom OpenCode command:

```text
/bb <authorized-target>
```

All other slash commands remain native OpenCode commands.

## Install

### Windows

Install Git, Python 3.11–3.13, and PowerShell. Then run:

```powershell
git clone https://github.com/pamungkasxd02-star/Yteam.git Yteam
cd Yteam
python scripts\install_yteam.py
```

The installer automatically:

1. downloads only the pinned runtime source needed by OpenCode, Hermes Agent,
   and Cybermes; tests, docs, website, desktop, and full knowledge corpora are
   not fetched by default;
2. creates `vendor\hermes-agent\.venv`;
3. installs Hermes dependencies;
4. installs the single root `requirements.txt` dependencies;
5. installs Bun if it is missing;
6. runs `bun install` for OpenCode;
7. fetches the Camoufox browser runtime;
8. installs the `yteam` launcher in the user bin directory.

Open a new terminal after the installer prints the PATH command.

Contributor/CI mode is available when the complete upstream source tree is
needed:

```powershell
python scripts\install_yteam.py --full-sources
```

Normal users should keep the default lean runtime profile.

### macOS/Linux

Install Git, Python 3.11–3.13, and a POSIX shell. Then run:

```bash
git clone https://github.com/pamungkasxd02-star/Yteam.git Yteam
cd Yteam
python3 scripts/install_yteam.py
```

The same source, Hermes, Bun, OpenCode, Camoufox, and launcher setup is used.

To see the plan without installing anything:

```bash
python scripts/install_yteam.py --dry-run
```

To install Camoufox without downloading its browser binary:

```bash
python scripts/install_yteam.py --skip-browser-download
```

Dependency files:

```text
requirements.txt          # all direct YTEAM Python dependencies, including Camoufox
```

## Model

YTEAM starts with **OpenCode Zen Free automatically**. No account, API key, or
local model file is required. The launcher detects the current free-model list
from OpenCode Zen and exposes it through the native OpenCode `/models` picker.
It uses the upstream Hermes `opencode-free` provider—not `opencode-go`—with a
safe default model:

```text
provider: opencode-free
model: laguna-s-2.1-free
endpoint: https://opencode.ai/zen/v1
```

The free choices include models such as `big-pickle`, `mimo-v2.5-free`,
`deepseek-v4-flash-free`, and `laguna-s-2.1-free` when advertised by Zen. Use
`/models` in the TUI to switch them. If Zen's catalog is temporarily
unreachable, YTEAM uses a bundled free-model fallback list.

Start immediately after installation:

```text
yteam
```

### Optional model override

Only copy the local-only template if you want to override the free default or
use a paid/other provider:

```powershell
Copy-Item .\yteam.local.example.yaml .\yteam.local.yaml
notepad .\yteam.local.yaml
```

Set the provider, model, key, and endpoint:

```yaml
provider: opencode-free
model: laguna-s-2.1-free
api_key: ""
base_url: "https://opencode.ai/zen/v1"
```

`yteam.local.yaml` is ignored by Git. The launcher writes only non-secret model
routing to the active Hermes profile and passes a paid-provider API key to
Hermes at runtime. The free provider remains keyless. Do not commit the local
file.

The model name shown in the TUI is **YTEAM**. The internal OpenCode model ID is
`yteam/yteam-agent`; the launcher maps the selected free model to Hermes
`opencode-free` automatically. If `yteam.local.yaml` exists, its provider/model
values override the default.
If you choose OpenRouter, paid Zen, or another provider, put its API key in the
same `api_key` field. The free default does not require this file or a key.

## Run

Start YTEAM:

```text
yteam
```

Then run the only YTEAM command:

```text
/bb https://authorized-target.example
```

The pipeline is:

```text
scope → toolchain → recon → Botterdop/Camoufox → hidden-surface analysis
      → hypothesis → safe validation → evidence → triage
```

The system is read-only by default, rate-limited, scope-gated, and never
auto-submits reports. `PACK`, `CAND`, `MID`, `BLOCKED`, and `0` are evidence
states, not guesses.

## Botterdop

Botterdop detects Cloudflare, Akamai, DataDome, Kasada, PerimeterX/HUMAN,
reCAPTCHA, Turnstile, WAF blocks, and `429` responses. It slows down or stops
when a gate is detected. Camoufox is an optional isolated browser observer; it
does not solve CAPTCHA, evade WAFs, rotate identities, or rotate proxies.

## Output

Runtime data stays local and is ignored by Git:

```text
runtime/bb-runs/<run-id>/<target>/
runtime/assessments/<run-id>/<target>/
```

Useful files include:

```text
scope.json
recon/recon.json
hidden_surface.json
hypotheses.json
hunt_context.md
assessment_manifest.json
```

## Troubleshooting

Run the local environment check:

```powershell
python scripts\yteam_doctor.py --json
```

If Bun is missing, install it and rerun:

```powershell
powershell -c "irm bun.sh/install.ps1 | iex"
```

If Camoufox is missing:

```powershell
python -m pip install camoufox
python -m camoufox fetch
```

## Requirements and legal use

Read [`REQUIREMENTS.md`](REQUIREMENTS.md) before installing. Publishing notes
are in [`docs/PUBLISHING.md`](docs/PUBLISHING.md).

Use YTEAM only on systems you own or are explicitly authorized to test. Do not
use it for credential stuffing, unauthorized access, destructive testing,
denial of service, persistence, or customer-data access.

## License

YTEAM integration code is MIT-licensed. See `THIRD_PARTY_NOTICES.md` for the
licenses of the upstream projects fetched by the installer.
