#!/usr/bin/env python3
"""Run the portable multi-stage Yteam/Cybermes web hunting pipeline."""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path
from urllib.parse import urlparse

from yteam_safety import redact_text, redact_value


ROOT = Path(__file__).resolve().parents[1]
WORKSPACE = ROOT.parent
DEFAULT_RUNS = ROOT / "runtime" / "bb-runs"
ATTRIBUTION = "pamungkas"
SECRET_RE = re.compile(r"(?i)(bearer\s+|authorization\s*[:=]\s*(?:bearer\s+)?|cookie\s*[:=]\s*|set-cookie\s*[:=]\s*|(?:api[_-]?key|token|password|secret)\s*[:=]\s*)([^\s,;]+)")
KNOWN_STAGES = ("scope", "inventory", "passive_assets", "baseline", "crawl", "route_mining", "candidate_validation", "evidence", "triage")
PIPELINE_PHASES = {"scope": "scope", "inventory": "recon", "passive_assets": "recon", "baseline": "recon", "crawl": "recon", "route_mining": "mapping", "candidate_validation": "validation", "evidence": "triage", "triage": "triage"}
TRACKS = {
    "web-surface": {"purpose": "HTTP, HTML, JS, API, docs, forms, and route mapping", "markers": (), "safe_default": "read-only baseline and bounded crawl"},
    "authorization": {"purpose": "IDOR/BOLA, function-level access, tenant, and role boundary analysis", "markers": ("/api/", "/v1", "/v2", "/users", "/account", "/admin", "/graphql"), "safe_default": "researcher-owned identities and synthetic IDs only"},
    "authentication": {"purpose": "Login, session, OAuth/OIDC, MFA, reset, and token boundary analysis", "markers": ("/auth", "/login", "/oauth", "/sso", "/session", "/token", "/reset"), "safe_default": "no credential stuffing; differential requests only"},
    "input-validation": {"purpose": "SQLi, NoSQLi, XSS, SSRF, upload, template, and parser hypotheses", "markers": ("?", "/search", "/query", "/fetch", "/proxy", "/upload", "/render", "/import", "/webhook"), "safe_default": "one controlled canary per candidate; no destructive payloads"},
    "business-logic": {"purpose": "Payment, order, invite, coupon, export, and state-machine review", "markers": ("/order", "/checkout", "/payment", "/invoice", "/refund", "/coupon", "/invite", "/export"), "safe_default": "synthetic researcher-owned objects and minimum writes"},
    "cloud-and-infra": {"purpose": "Cloud, storage, admin panels, service exposure, and takeover signals", "markers": ("/swagger", "/openapi", "/actuator", "/debug", "/internal", "/metrics", "/.git", "/.env"), "safe_default": "metadata/read-only checks; no resource claims"},
    "client-and-browser": {"purpose": "CORS, CSRF, DOM, postMessage, browser and screenshot workflows", "markers": ("/callback", "/redirect", "/preview", "/screenshot", "/browser", "/graphql"), "safe_default": "browser proof only on authorized test fixtures"},
    "reporting": {"purpose": "Evidence hygiene, triage, severity, aggregation, and learning", "markers": (), "safe_default": "redact secrets and never auto-submit"},
}


def slugify(value: str) -> str:
    parsed = urlparse(value if "://" in value else f"https://{value}")
    return re.sub(r"[^a-zA-Z0-9]+", "_", parsed.netloc or parsed.path).strip("_").lower()[:96] or "target"


def normalize_target(value: str) -> str:
    value = value.strip()
    if "://" not in value:
        value = "https://" + value
    parsed = urlparse(value)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise ValueError("target must be an HTTP(S) URL or hostname")
    return value.rstrip("/")


def locate(name: str) -> str | None:
    suffix = ".exe" if os.name == "nt" else ""
    for path in (ROOT / "runtime" / "bin" / f"{name}{suffix}", ROOT / "vendor" / "cybermes" / "tools" / "bin" / f"{name}{suffix}"):
        if path.exists() and path.is_file():
            return str(path)
    return shutil.which(name)


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(redact_value(value), indent=2) + "\n", encoding="utf-8")


def redact_output(value: str) -> str:
    return redact_text(value)


def build_track_plan(target: str, recon_path: Path) -> dict:
    routes: list[dict] = []
    technologies: list[str] = []
    if recon_path.exists():
        try:
            data = json.loads(recon_path.read_text(encoding="utf-8"))
            if isinstance(data, dict):
                routes = data.get("routes", [])
                technologies = [str(item).lower() for item in data.get("technology", [])]
        except (OSError, json.JSONDecodeError):
            pass
    haystack = " ".join([str(item.get("url", "")).lower() for item in routes if isinstance(item, dict)] + technologies)
    plan: list[dict] = []
    for name, spec in TRACKS.items():
        matched = [marker for marker in spec["markers"] if marker and marker.lower() in haystack]
        eligible = name in {"web-surface", "reporting"} or bool(matched)
        plan.append({
            "track": name,
            "purpose": spec["purpose"],
            "status": "eligible" if eligible else "planned",
            "signals": matched,
            "prerequisite": "written scope + exact target" if name != "reporting" else "evidence or recon output",
            "safe_default": spec["safe_default"],
            "next_action": "load narrow matching skills and run the smallest proof" if eligible else "activate only if later recon reveals a matching signal",
        })
    return {"schema_version": 1, "target": target, "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "tracks": plan, "non_claim": "Track eligibility is not vulnerability proof."}


def run_external(name: str, args: list[str], output: Path, timeout: int, env: dict[str, str]) -> dict:
    command = locate(name)
    started = time.monotonic()
    result = {"tool": name, "available": command is not None, "status": "missing", "command": args, "elapsed_ms": 0, "stdout_path": str(output), "stderr_path": str(output.with_suffix(".stderr.txt"))}
    output.parent.mkdir(parents=True, exist_ok=True)
    if command is None:
        output.write_text(f"{name} not available; native fallback or manual review required.\n", encoding="utf-8")
        result["elapsed_ms"] = int((time.monotonic() - started) * 1000)
        return result
    full = [command, *args]
    safe_env = env.copy()
    safe_env["X_BUG_BOUNTY"] = ATTRIBUTION
    try:
        completed = subprocess.run(full, cwd=ROOT, env=safe_env, capture_output=True, text=True, errors="replace", timeout=timeout, check=False)
        output.write_text(redact_output(completed.stdout), encoding="utf-8")
        output.with_suffix(".stderr.txt").write_text(redact_output(completed.stderr), encoding="utf-8")
        result["status"] = "completed" if completed.returncode == 0 else "nonzero"
        result["returncode"] = completed.returncode
    except subprocess.TimeoutExpired as error:
        output.write_text(redact_output(error.stdout) if isinstance(error.stdout, str) else "", encoding="utf-8")
        output.with_suffix(".stderr.txt").write_text("timeout\n", encoding="utf-8")
        result["status"] = "timeout"
    result["elapsed_ms"] = int((time.monotonic() - started) * 1000)
    return result


def append_pipeline_event(run_id: str | None, kind: str, detail: str, phase: str) -> None:
    if not run_id:
        return
    sys.path.insert(0, str(ROOT / "scripts"))
    try:
        from bb_pipeline import event

        event(run_id, kind, detail, phase)
    finally:
        sys.path.remove(str(ROOT / "scripts"))


def run(target: str, output: Path, run_id: str | None, depth: int, rate: float, use_external: bool, scan: bool, scope_file: Path | None = None) -> dict:
    target = normalize_target(target)
    depth = max(1, min(int(depth), 3))
    rate = max(0.1, min(float(rate), 10.0))
    output = output or (ROOT / "runtime" / "recon" / slugify(target))
    output.mkdir(parents=True, exist_ok=True)
    sys.path.insert(0, str(ROOT / "scripts"))
    try:
        from yteam_scope import validate

        scope = validate(target, slugify(target), scope_file)
    finally:
        sys.path.remove(str(ROOT / "scripts"))
    manifest = {
        "schema_version": 1,
        "engine": "yteam-hunt",
        "target": target,
        "target_slug": slugify(target),
        "run_id": run_id,
        "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "attribution_header": f"X-Bug-Bounty: {ATTRIBUTION}",
        "scope": scope.__dict__,
        "policy": {"read_only": True, "rate_limit_rps": rate, "max_depth": depth, "external_tools": use_external, "optional_validation_scan": scan, "no_destructive_actions": True},
        "stages": [],
        "tool_runs": [],
        "non_claims": ["Recon and scanner output are not vulnerability proof.", "Passive assets are not active targets until scope approval.", "A hypothesis is not a finding."],
    }

    def stage(name: str, status: str, detail: str) -> None:
        manifest["stages"].append({"name": name, "status": status, "detail": detail, "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())})
        append_pipeline_event(run_id, "note" if status != "blocked" else "blocker", detail, PIPELINE_PHASES.get(name, "scope"))

    write_json(output / "scope.json", scope.__dict__)
    if not scope.allowed:
        stage("scope", "blocked", scope.reason)
        manifest["status"] = "blocked"
        manifest["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        write_json(output / "hunt.json", manifest)
        return manifest
    stage("scope", "completed", f"Scope gate passed ({scope.mode}): {scope.reason}")
    stage("inventory", "active", "Discovering available native and optional tools without installing or modifying the host.")
    inventory_script = ROOT / "scripts" / "yteam_toolchain.py"
    inventory = subprocess.run([sys.executable, str(inventory_script), "--json"], capture_output=True, text=True, check=False)
    (output / "toolchain.json").write_text(inventory.stdout, encoding="utf-8")
    stage("inventory", "completed", "Toolchain inventory saved to toolchain.json.")

    hostname = urlparse(target).hostname or ""
    if use_external and hostname and "." in hostname:
        stage("passive_assets", "active", "Running passive subdomain discovery; discovered names will not be probed automatically.")
        manifest["tool_runs"].append(run_external("subfinder", ["-d", hostname, "-silent", "-timeout", "10"], output / "subfinder.txt", 45, os.environ.copy()))
        manifest["tool_runs"].append(run_external("gau", [hostname, "--subs", "--blacklist", "png,jpg,jpeg,gif,svg,css,woff,woff2"], output / "gau.txt", 90, os.environ.copy()))
        manifest["tool_runs"].append(run_external("waybackurls", [hostname], output / "waybackurls.txt", 90, os.environ.copy()))
        stage("passive_assets", "completed", "Passive subdomain output archived; active probing remains scope-gated.")
    else:
        stage("passive_assets", "skipped", "No external passive tool selected or target is not a domain.")

    stage("baseline", "active", "Running native Yteam baseline, headers, docs, HTML/JS, form, route, DNS, and bounded crawl mapping.")
    recon_script = ROOT / "scripts" / "yteam_recon.py"
    recon_args = [sys.executable, str(recon_script), "run", "--target", target, "--output", str(output), "--depth", str(depth), "--rate", str(rate)]
    if run_id:
        recon_args.extend(["--run-id", run_id])
    recon_env = os.environ.copy()
    if run_id:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            import bb_pipeline

            recon_env["YTEAM_BB_RUNS"] = str(bb_pipeline.RUNS)
        finally:
            sys.path.remove(str(ROOT / "scripts"))
    recon = subprocess.run(recon_args, capture_output=True, text=True, env=recon_env, check=False)
    (output / "yteam_recon.stdout.txt").write_text(recon.stdout, encoding="utf-8")
    (output / "yteam_recon.stderr.txt").write_text(recon.stderr, encoding="utf-8")
    if recon.returncode == 0:
        stage("baseline", "completed", "Native Yteam recon completed.")
    else:
        stage("baseline", "blocked", "Native Yteam recon failed; inspect yteam_recon.stderr.txt before continuing.")
        manifest["status"] = "blocked"
        manifest["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        write_json(output / "hunt.json", manifest)
        return manifest

    track_plan = build_track_plan(target, output / "recon.json")
    write_json(output / "track_plan.json", track_plan)
    eligible_tracks = [item["track"] for item in track_plan["tracks"] if item["status"] == "eligible"]
    stage("route_mining", "completed", f"Adaptive track plan saved; eligible tracks: {', '.join(eligible_tracks)}.")

    skill_script = ROOT / "scripts" / "yteam_skills.py"
    skill_registry = output / "cybermes-skill-registry.json"
    skill_index = subprocess.run([sys.executable, str(skill_script), "index", "--output", str(skill_registry)], capture_output=True, text=True, check=False)
    (output / "skill-index.stdout.txt").write_text(skill_index.stdout, encoding="utf-8")
    (output / "skill-index.stderr.txt").write_text(skill_index.stderr, encoding="utf-8")
    skill_signals = [target, *eligible_tracks]
    try:
        recon_for_skills = json.loads((output / "recon.json").read_text(encoding="utf-8"))
        skill_signals.extend(str(item) for item in recon_for_skills.get("technology", []))
        skill_signals.extend(str(item.get("url", "")) for item in recon_for_skills.get("routes", [])[:80] if isinstance(item, dict))
    except (OSError, json.JSONDecodeError):
        pass
    bundle = subprocess.run([sys.executable, str(skill_script), "bundle", "--signals", *skill_signals, "--limit", "24"], capture_output=True, text=True, check=False)
    (output / "skill-bundle.stdout.txt").write_text(bundle.stdout, encoding="utf-8")
    (output / "skill-bundle.stderr.txt").write_text(bundle.stderr, encoding="utf-8")
    try:
        bundle_data = json.loads(bundle.stdout)
    except json.JSONDecodeError:
        bundle_data = {"selected_count": 0, "skills": [], "signals": skill_signals}
    write_json(output / "cybermes-skill-bundle.json", bundle_data)
    stage("route_mining", "completed", f"Complete Cybermes skill registry and adaptive bundle saved; selected {bundle_data.get('selected_count', 0)} skills.")

    intelligence_script = ROOT / "scripts" / "yteam_intelligence.py"
    observations = output / "intelligence" / "observations.jsonl"
    hypotheses = output / "hypotheses.json"
    if observations.exists():
        analysis = subprocess.run([sys.executable, str(intelligence_script), "analyze", "--ledger", str(observations), "--output", str(hypotheses)], capture_output=True, text=True, check=False)
        (output / "intelligence.stdout.txt").write_text(analysis.stdout, encoding="utf-8")
        (output / "intelligence.stderr.txt").write_text(analysis.stderr, encoding="utf-8")
    else:
        write_json(hypotheses, {"observation_count": 0, "hypothesis_count": 0, "hypotheses": []})

    recon_data = {}
    try:
        recon_data = json.loads((output / "recon.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        pass

    hidden_output = output / "hidden_surface.json"
    hidden_script = ROOT / "scripts" / "yteam_hidden.py"
    hidden_run = subprocess.run(
        [sys.executable, str(hidden_script), str(output / "recon.json"), "--target", target, "--output", str(hidden_output), "--limit", "80"],
        capture_output=True,
        text=True,
        check=False,
    )
    (output / "hidden-surface.stdout.txt").write_text(redact_output(hidden_run.stdout), encoding="utf-8")
    (output / "hidden-surface.stderr.txt").write_text(redact_output(hidden_run.stderr), encoding="utf-8")
    if hidden_run.returncode == 0 and hidden_output.exists():
        try:
            hidden_data = json.loads(hidden_output.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            hidden_data = {"hypothesis_count": 0, "hypotheses": [], "safe_checks": []}
        stage("route_mining", "completed", f"Hidden-surface analysis saved; {hidden_data.get('hypothesis_count', 0)} trust-boundary hypotheses generated.")
    else:
        hidden_data = {"hypothesis_count": 0, "hypotheses": [], "safe_checks": [], "status": "degraded"}
        stage("route_mining", "degraded", "Hidden-surface analysis was unavailable; native recon and adaptive track planning remain authoritative.")
    top_routes = [item for item in recon_data.get("routes", []) if isinstance(item, dict)][:30]
    next_actions = [
        {"order": index + 1, "track": item["track"], "status": item["status"], "signals": item["signals"], "action": item["next_action"], "safe_default": item["safe_default"]}
        for index, item in enumerate(track_plan["tracks"])
        if item["status"] == "eligible"
    ]
    context = {
        "schema_version": 1,
        "engine": "yteam-hunt",
        "target": target,
        "run_id": run_id,
        "scope": manifest["scope"],
        "policy": manifest["policy"],
        "available_tools": json.loads((output / "toolchain.json").read_text(encoding="utf-8")) if (output / "toolchain.json").exists() else [],
        "skill_registry_path": str(skill_registry),
        "skill_bundle_path": str(output / "cybermes-skill-bundle.json"),
        "selected_skill_count": bundle_data.get("selected_count", 0),
        "eligible_tracks": eligible_tracks,
        "next_actions": next_actions,
        "top_routes": top_routes,
        "hypotheses_path": str(hypotheses),
        "hidden_surface_path": str(hidden_output),
        "hidden_hypothesis_count": hidden_data.get("hypothesis_count", 0),
        "hidden_safe_checks": hidden_data.get("safe_checks", [])[:40],
        "required_llm_contract": ["Read scope.json, recon.json, track_plan.json, cybermes-skill-bundle.json, and hypotheses.json", "Select one eligible track and matching skill bundle", "Use researcher-owned fixtures and safe read-first proof", "Record observation/proof/blocker", "Run triage gate before any finding"],
        "non_claims": manifest["non_claims"],
    }
    write_json(output / "next_actions.json", next_actions)
    write_json(output / "hunt_context.json", context)
    context_lines = [
        "# Yteam Hunt Context", "", f"- Target: `{target}`", f"- Run ID: `{run_id or 'standalone'}`", f"- Scope: `{manifest['scope']['mode']}` — {manifest['scope']['reason']}", f"- Eligible tracks: {', '.join(eligible_tracks) or 'none'}", f"- Selected Cybermes skills: {bundle_data.get('selected_count', 0)}", "", "## Required model contract", "", "1. Read `scope.json`, `recon.json`, `hidden_surface.json`, `track_plan.json`, `cybermes-skill-bundle.json`, and `hypotheses.json`.", "2. Select one eligible track, one matching skill bundle, and one concrete impact objective.", "3. Use researcher-owned fixtures, read-first requests, and safe rate limits.", "4. Record proof, negative result, or blocker; never promote anomaly directly to finding.", "5. Run triage validation before creating any report.", "", "## Hidden-surface review", "", f"- Hypotheses: {hidden_data.get('hypothesis_count', 0)}", *(f"- `{item.get('id')}` [{item.get('track')}]: {item.get('class')} — {item.get('signal')}" for item in hidden_data.get('hypotheses', [])[:20]), "", "## Next actions", "", *(f"- **{item['track']}** ({item['status']}): {item['action']} — {item['safe_default']}" for item in next_actions), "", "## Top routes", "", *(f"- `{item.get('url')}` — priority {item.get('priority')} — {', '.join(item.get('reasons', []))}" for item in top_routes), "", "## Non-claims", "", *(f"- {item}" for item in manifest["non_claims"]), ""]
    (output / "hunt_context.md").write_text("\n".join(context_lines), encoding="utf-8")

    if use_external:
        stage("crawl", "active", "Running optional Katana crawl with bounded depth and low request rate.")
        manifest["tool_runs"].append(run_external("katana", ["-u", target, "-d", str(depth), "-jc", "-silent", "-rate-limit", str(max(1, min(5, int(rate * 5)))), "-H", f"X-Bug-Bounty: {ATTRIBUTION}", "-ct", "12s"], output / "katana.txt", 90, os.environ.copy()))
        stage("crawl", "completed", "Optional crawler output archived for route reconciliation.")
    else:
        stage("crawl", "skipped", "External crawler disabled; native crawler output is authoritative for this run.")

    stage("route_mining", "completed", "Routes, response fingerprints, forms, JS candidates, security headers, archive URLs, passive assets, and adaptive tracks are available in recon.json/routes.jsonl/track_plan.json.")
    if scan and use_external:
        stage("candidate_validation", "active", "Running limited verification templates only; no destructive or denial-of-service templates.")
        manifest["tool_runs"].append(run_external("nuclei", ["-u", target, "-silent", "-rate-limit", str(max(1, min(5, int(rate * 5)))), "-H", f"X-Bug-Bounty: {ATTRIBUTION}", "-tags", "cve,misconfig,exposure", "-severity", "medium,high,critical"], output / "nuclei.txt", 120, os.environ.copy()))
        stage("candidate_validation", "completed", "Verification output is candidate evidence only and requires manual proof.")
    else:
        stage("candidate_validation", "skipped", "No optional validation scan selected; hypothesis-driven validation remains in Hermes.")
    stage("evidence", "completed", "Raw outputs and redacted recon artifacts are target-scoped under the run directory.")
    stage("triage", "active", "Hermes must apply the seven-question gate before any report is written.")
    manifest["status"] = "ready_for_analysis"
    manifest["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    write_json(output / "hunt.json", manifest)
    if run_id:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from bb_pipeline import advance

            advance(run_id, "triage", "active")
        finally:
            sys.path.remove(str(ROOT / "scripts"))
    append_pipeline_event(run_id, "note", f"Deep hunt orchestration completed: {len(manifest['tool_runs'])} optional tool runs; status requires Hermes analysis.", "triage")
    return manifest


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run", choices=("run", "bundle_plan"))
    parser.add_argument("--target", required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--run-id")
    parser.add_argument("--depth", type=int, default=2)
    parser.add_argument("--rate", type=float, default=1.0)
    parser.add_argument("--no-external", action="store_true")
    parser.add_argument("--scan", action="store_true", help="Run limited verification templates; not enabled by default")
    parser.add_argument("--scope-file", type=Path)
    args = parser.parse_args()
    try:
        if args.run == "run":
            manifest = run(args.target, args.output, args.run_id, args.depth, args.rate, not args.no_external, args.scan, args.scope_file)
            output_path = args.output or (ROOT / "runtime" / "recon" / slugify(manifest["target"]))
            print(json.dumps({"target": manifest["target"], "status": manifest["status"], "output": str(output_path), "stages": len(manifest["stages"]), "optional_tools": len(manifest["tool_runs"])}, indent=2))
            return 0 if manifest["status"] != "blocked" else 2

        # bundle_plan: derive track plan + skill bundle from an existing
        # recon.json without re-running the full hunt pipeline.
        recon_path = Path(args.output) / "recon" / "recon.json" if args.output else ROOT / "runtime" / "recon" / "recon.json"
        if args.output and (Path(args.output) / "recon.json").exists():
            recon_path = Path(args.output) / "recon.json"
        if not recon_path.exists():
            print(f"yteam_hunt: recon.json not found at {recon_path}", file=sys.stderr)
            return 2
        track_plan = build_track_plan(args.target, recon_path)
        write_json(recon_path.parent / "track_plan.json", track_plan)
        eligible = [item["track"] for item in track_plan["tracks"] if item["status"] == "eligible"]
        skill_script = ROOT / "scripts" / "yteam_skills.py"
        signals = [args.target, *eligible]
        try:
            data = json.loads(recon_path.read_text(encoding="utf-8"))
            signals.extend(str(item) for item in data.get("technology", []))
            signals.extend(str(item.get("url", "")) for item in data.get("routes", [])[:80] if isinstance(item, dict))
        except (OSError, json.JSONDecodeError):
            pass
        bundle = subprocess.run([sys.executable, str(skill_script), "bundle", "--signals", *signals, "--limit", "24"], capture_output=True, text=True, check=False)
        try:
            bundle_data = json.loads(bundle.stdout)
        except json.JSONDecodeError:
            bundle_data = {"selected_count": 0, "skills": []}
        write_json(recon_path.parent / "cybermes-skill-bundle.json", bundle_data)
        print(json.dumps({"target": args.target, "eligible_tracks": eligible, "selected_skills": bundle_data.get("selected_count", 0), "track_plan": str(recon_path.parent / "track_plan.json"), "skill_bundle": str(recon_path.parent / "cybermes-skill-bundle.json")}, indent=2))
        return 0
    except ValueError as error:
        print(f"yteam_hunt: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
