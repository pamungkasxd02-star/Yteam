# Yteam Platform — Getting Started

## What this project is

Yteam is one security workbench with a Yteam-branded OpenCode TUI and a
Hermes-powered execution layer. The platform unifies five pillars:

1. bot/anti-automation gate detection for authorized QA;
2. encoded/encrypted response and signature format analysis for authorized reverse engineering;
3. pentest and application-security QA;
4. server hardening and exposure review;
5. bug-bounty reconnaissance, validation, intelligence, and reporting.

## Install from a GitHub clone

### Windows PowerShell

```powershell
git clone <YOUR_YTEAM_REPOSITORY_URL> yteam
cd yteam
python scripts\bootstrap_sources.py
cd vendor\hermes-agent
uv venv --python 3.11 .venv
uv pip install -e ".[all]"
cd ..\opencode
bun install
cd ..\..
python scripts\install_yteam.py
```

Open a new PowerShell window after adding the printed bin directory to PATH.

### macOS/Linux

```bash
git clone <YOUR_YTEAM_REPOSITORY_URL> yteam
cd yteam
python3 scripts/bootstrap_sources.py
cd vendor/hermes-agent
uv venv --python 3.11 .venv
uv pip install -e ".[all]"
cd ../opencode
bun install
cd ../..
chmod +x yteam scripts/hermes_opencode.sh
python3 scripts/install_yteam.py
```

`bootstrap_sources.py` keeps OpenCode, Hermes Agent, and Cybermes as separate
upstream checkouts. Cybermes uses a sparse checkout to avoid Windows filename
length failures while retaining its engine, skills, docs, tools, and metadata.

## Configure a model

Put the model and API key in one local `yteam.local.yaml` file, never in
`opencode.json` or a report. Create it from `yteam.local.example.yaml`:

```powershell
Copy-Item .\yteam.local.example.yaml .\yteam.local.yaml
notepad .\yteam.local.yaml
```

Fill in `provider`, `model`, `api_key`, and optionally `base_url`. The file is
ignored by Git. The launcher writes only non-secret model routing into the
active Hermes profile and passes the key ephemerally to Hermes. The default
profile is created on first launch at:

```text
runtime/yteam-hermes-home/
```

After filling the file, run:

```powershell
python .\scripts\yteam_doctor.py --json
yteam
```

No separate `.env` file is needed for the main model in this mode.

It contains:

```text
SOUL.md
config.yaml
memories/MEMORY.md
memories/USER.md
```

For a durable profile elsewhere:

```powershell
$env:YTEAM_HERMES_HOME = "D:\Yteam\profile"
yteam
```

The optional `vision` block in `yteam.local.yaml` documents the auxiliary model;
configure the corresponding auxiliary section in the active profile when using
it. Yteam will use native vision when available, then
Hermes auxiliary vision/OCR fallback, and will never invent visual results.

## Start the TUI

```text
yteam
```

The TUI is a normal chat interface. In it, use the single autonomous command:

```text
/bb https://authorized-target.example
```

The `/bb` command includes the optional isolated Camoufox observation pass for
Botterdop. If Camoufox is not installed, Yteam records `camoufox: unavailable`
and keeps the native HTTP Botterdop detector active; it does not silently claim
that browser verification happened.

The user does not need to run the internal scripts manually. `/bb` performs:

```text
queue/scope → toolchain → recon → skill/track routing → hypothesis →
```

The `/bb` path also enables the optional Camoufox observation pass when the
package and browser runtime are installed. If Camoufox is unavailable, Yteam
records that status and continues with native HTTP Botterdop detection; it does
not claim that browser verification happened.

The run creates a target-scoped artifact directory under:

```text
runtime/assessments/<run-id>/<target-slug>/
```

Read `assessment_context.md` for the compact model context. Raw tool output is
kept on disk and is not dumped into the conversation.

Every `/bb` run also creates `hidden_surface.json`. It contains a ranked,
bug-first review plan for object references, sibling endpoint inconsistencies,
API version drift, GraphQL/REST overlap, URL-processing flows, authentication
surfaces, and business-state transitions. These remain hypotheses until a
researcher-owned, read-only validation crosses a real security boundary.

## Custom command inside chat

```text
/bb <target>      full autonomous assessment
```

YTEAM intentionally registers only `/bb`. All other slash commands remain
native OpenCode commands. Internal helpers such as `yteam_doctor.py` and
`yteam_hidden.py` are invoked by `/bb` when appropriate or run directly from
the terminal; they are not extra custom TUI commands.

## Direct command-line diagnostics

```powershell
python scripts\yteam_toolchain.py --json
python scripts\yteam_engine.py --list-engines
python scripts\yteam_skills.py index --output runtime\cybermes-skill-registry.json
python scripts\yteam_skills.py bundle --signals api graphql authorization --limit 24
python scripts\yteam_assessment.py https://authorized-target.example
```

## Output and evidence

Every assessment keeps its own:

```text
scope.json
toolchain.json
recon/
assessment_manifest.json
assessment_context.md
assessment_context.json
```

Reports and PoCs are English. Redact cookies, bearer tokens, passwords, API
keys, unrelated PII, and raw customer data. Do not interpret a scanner match,
bot gate, missing header, status difference, or decoded format as a finding
without reproducible impact and triage validation.

## Botterdop/Camoufox setup

Camoufox is optional and is used only for isolated authorized browser
observation. Install it in the active environment and keep its browser cache
on the data drive:

```powershell
python -m pip install camoufox
python -m camoufox fetch
```

Run the adapter directly with `python scripts\botterdop.py <authorized-target>`.
Use `--headed` only for operator-controlled manual review. It never solves
CAPTCHA or attempts WAF/fingerprint evasion.

## Optional Camoufox browser setup

Camoufox is only needed for browser-side Botterdop observation and authorized
manual review. Install it inside the active Hermes/Yteam Python environment,
not into the system Python:

```powershell
python -m pip install camoufox
python -m camoufox fetch
```

Keep the browser cache on the data drive when configuring the environment, for
example by setting the cache variable supported by the installed Camoufox/
Playwright release to a directory under `D:\Yteam\cache\camoufox`. Do not use a
personal browser profile. Then `/bb <target>` will record the browser response
observations under the target's `camoufox/` artifact directory.

To run the adapter directly without opening the TUI:

```powershell
python scripts\botterdop.py https://authorized-target.example --output D:\Yteam\runtime\botterdop
python scripts\botterdop.py https://authorized-target.example --headed
```

`--headed` is for an operator-controlled, authorized manual review only. It
does not enable challenge solving or WAF evasion.

## Native OpenCode commands

The Yteam launcher preserves upstream OpenCode commands. For example:

```text
yteam --version
```

The default bare `yteam` path is the Yteam bridge + TUI chat path; explicit
native OpenCode commands are passed to the upstream source CLI.
