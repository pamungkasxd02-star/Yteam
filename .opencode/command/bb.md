---
description: Run the complete autonomous Yteam bug-bounty and recon pipeline.
agent: yteam-security
---

You are the autonomous Yteam orchestration driver. `$ARGUMENTS` is the optional target.

Run the whole unified multi-pillar pipeline with ONE command through Hermes `terminal`:

```text
python scripts/yteam_run.py --camoufox $ARGUMENTS
```

`yteam_run.py` automatically invokes the unified control plane: all security pillars share one assessment context, policy, event bus, artifact store, and DAG. The `--camoufox` flag enables the isolated Camoufox observation pass for Botterdop; if Camoufox is unavailable, the run records that blocker and continues with native HTTP detection. No helper command is required from the operator.

If `$ARGUMENTS` is empty, `yteam_run.py` performs queue triage: it resumes the most recent in-progress run, or points at `bugbounty_meta/` to inspect the queue/locks before selecting a target. Never invent a sibling domain or an asset outside the exact written scope.

Then do NOT re-run helpers manually. Read the driver result and act on it:

- If `ok: true` with `status`/`phase`: read `assessment_context.md` and `assessment_context.json`, then follow the model action contract on the selected tracks. The control plane already ran scope, toolchain, recon, bot-gate, decrypt-format, pentest/QA, server-guard, intelligence, learning, and delivery engines in one shared run.
- If `ok: false`: surface the `error` and `command` that blocked, record it in the ledger, and stop with `BLOCKED` unless a safe next step is obvious.
- Resume an existing run with `--resume <run-id>` instead of starting a new one.

Execute these phases in one continuous deep-one run:

1. **Scope and queue** — reject OOS, duplicate, locked, unavailable, or customer-object targets. Read scope/locks/aliases/prior reports before any live probe.
2. **Recon and surface** — the engine runs tool inventory, passive assets, DNS, HTTP/TLS/header fingerprint, robots/sitemap, HTML/JS/API docs, forms, bounded crawl, dedup, and priority scoring.
3. **Map and prioritize** — activate only eligible tracks (web-surface, authorization, authentication, input-validation, business-logic, cloud-and-infra, client-and-browser, reporting) and select the target-relevant Cybermes skill bundle.
4. **Hypothesis and validation** — use the ranked hypotheses from `hypotheses.json`; test with safe low-rate requests and researcher-owned fixtures. A hypothesis is not a finding.
5. **Impact proof** — only the minimum non-destructive proof that crosses a real security boundary (e.g. CORS browser-readable private data, attributed SSRF, cross-identity IDOR, current-browser XSS impact, concrete injection).
6. **Triage and deliverable** — run the seven-question gate, redact evidence, write English PoC/report only for confirmed findings, and update the ledger to `PACK`/`CAND`/`MID`/`BLOCKED`/`0`.

Keep Hermes as the single tool owner. Cybermes is consumed directly via `scripts/cybermes.py`; never start a Cybermes MCP server. Stay low-rate, stop on 429/WAF/bot gates, never credential-stuff, never access customer objects, never claim cloud resources, never destructive/DoS/persistence, and never auto-submit.

At the end print a compact status: target, run ID, phase, `PACK/CAND/MID/BLOCKED/0`, evidence/report path, exact blocker or next action, and hypothesis count. Save durable non-secret lessons through Hermes memory for later runs.
