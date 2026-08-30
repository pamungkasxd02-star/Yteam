#!/usr/bin/env python3
"""Autonomous Yteam /bb driver.

One command drives the whole authorized hunt: queue triage (empty) or a
concrete target, then prepare -> hunt -> intelligence -> status. The model
reads the returned run context and does not need to run helpers manually.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_RUNS = Path(os.environ.get("YTEAM_BB_RUNS", ROOT / "runtime" / "bb-runs"))


def _py() -> str:
    return sys.executable


def _run(cmd: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
    return subprocess.run(cmd, capture_output=True, text=True, errors="replace", check=False, **kwargs)


def run_script(name: str, args: list[str], env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return _run([_py(), str(ROOT / "scripts" / name), *args], env=env or os.environ.copy())


def list_active_runs() -> list[dict]:
    if not DEFAULT_RUNS.exists():
        return []
    runs: list[dict] = []
    for path in sorted(DEFAULT_RUNS.glob("bb_*.json"), reverse=True):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            continue
        if isinstance(data, dict) and data.get("status") in {"active", "ready_for_analysis"}:
            runs.append({"run_id": data.get("run_id"), "target": data.get("target"), "phase": data.get("current_phase"), "status": data.get("status"), "updated_at": data.get("updated_at")})
    return runs


def queue_triage() -> dict:
    """Pick the most recent in-progress run, or a target from bugbounty_meta.

    Stays non-destructive: it only selects a target/run, it does not start
    active testing itself.
    """
    runs = list_active_runs()
    if runs:
        return {"mode": "resume", "selection": runs[0]}
    meta = ROOT.parent / "bugbounty_meta"
    if meta.exists():
        return {"mode": "queue", "note": "bugbounty_meta present; inspect queue and locks before selecting a target.", "selection": None}
    return {"mode": "queue", "note": "No active run and no bugbounty_meta found; provide a target to /bb.", "selection": None}


def autonomous_run(target: str, depth: int, rate: float, scan: bool, scope_file: Path | None = None) -> dict:
    env = os.environ.copy()
    env.setdefault("YTEAM_BB_RUNS", str(DEFAULT_RUNS))
    prepare = run_script("bb_pipeline.py", ["prepare", "--target", target], env=env)
    if prepare.returncode != 0:
        return {"ok": False, "error": prepare.stderr.strip() or prepare.stdout.strip(), "command": "prepare"}
    ledger = json.loads(prepare.stdout)
    run_id = str(ledger["run_id"])
    run_dir = ROOT / ledger["paths"]["recon"]
    hunt = run_script("yteam_hunt.py", ["run", "--target", target, "--run-id", run_id, "--output", str(run_dir), "--depth", str(depth), "--rate", str(rate), *(["--scan"] if scan else []), *(["--scope-file", str(scope_file)] if scope_file else [])], env=env)
    if hunt.returncode != 0:
        # Still surface the ledger so the user can resume.
        status = run_script("bb_pipeline.py", ["status", "--run-id", run_id], env=env)
        try:
            ledger = json.loads(status.stdout)
        except json.JSONDecodeError:
            pass
        return {"ok": False, "run_id": run_id, "ledger": ledger, "error": hunt.stderr.strip() or hunt.stdout.strip(), "command": "hunt"}
    try:
        hunt_manifest = json.loads((run_dir / "hunt.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        hunt_manifest = {}
    context = {}
    try:
        context = json.loads((run_dir / "hunt_context.json").read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        pass
    return {
        "ok": True,
        "run_id": run_id,
        "target": target,
        "target_slug": ledger.get("target_slug"),
        "status": hunt_manifest.get("status"),
        "phase": ledger.get("current_phase"),
        "eligible_tracks": context.get("eligible_tracks", []),
        "selected_skills": context.get("selected_skill_count", 0),
        "hypotheses_path": str(run_dir / "hypotheses.json"),
        "context_path": str(run_dir / "hunt_context.md"),
        "ledger_path": str(DEFAULT_RUNS / f"{run_id}.json"),
        "next": "Read hunt_context.md and run the model action contract on the selected track(s).",
    }


def resume(run_id: str) -> dict:
    env = os.environ.copy()
    env.setdefault("YTEAM_BB_RUNS", str(DEFAULT_RUNS))
    status = run_script("bb_pipeline.py", ["status", "--run-id", run_id], env=env)
    if status.returncode != 0:
        return {"ok": False, "error": status.stderr.strip() or status.stdout.strip(), "command": "status"}
    ledger = json.loads(status.stdout)
    run_dir = ROOT / ledger["paths"]["recon"]
    return {
        "ok": True,
        "run_id": run_id,
        "target": ledger.get("target"),
        "phase": ledger.get("current_phase"),
        "status": ledger.get("status"),
        "context_path": str(run_dir / "hunt_context.md") if (run_dir / "hunt_context.md").exists() else None,
        "ledger_path": str(DEFAULT_RUNS / f"{run_id}.json"),
        "next": "Continue from the current phase; read the context or ledger before proceeding.",
    }


def engine_run(target: str, run_id: str | None = None) -> dict:
    """Run the DAG-driven multi-engine orchestrator."""
    env = os.environ.copy()
    env.setdefault("YTEAM_BB_RUNS", str(DEFAULT_RUNS))
    cmd = [_py(), str(ROOT / "scripts" / "yteam_engine.py")]
    if run_id:
        cmd += ["--resume", run_id]
    else:
        cmd += [target]
    result = _run(cmd, env=env)
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError:
        payload = {"ok": False, "error": result.stderr.strip() or result.stdout.strip()}
    return payload


def unified_run(target: str, scope_file: Path | None = None, rate: float = 1.0, scan: bool = False, camoufox: bool = False) -> dict:
    """Run the unified five-pillar Yteam control plane."""
    env = os.environ.copy()
    env.setdefault("YTEAM_ASSESSMENTS_ROOT", str(ROOT / "runtime" / "assessments"))
    cmd = [_py(), str(ROOT / "scripts" / "yteam_assessment.py"), target, "--rate", str(rate)]
    if scope_file:
        cmd += ["--scope-file", str(scope_file)]
    if scan:
        cmd.append("--scan")
    if camoufox:
        cmd.append("--camoufox")
    result = _run(cmd, env=env)
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        return {"ok": False, "error": result.stderr.strip() or result.stdout.strip(), "command": "yteam_assessment"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target", nargs="?", default="")
    parser.add_argument("--resume", metavar="RUN_ID")
    parser.add_argument("--engine", action="store_true", help="Use the unified multi-pillar Yteam control plane")
    parser.add_argument("--depth", type=int, default=2)
    parser.add_argument("--rate", type=float, default=1.0)
    parser.add_argument("--scan", action="store_true")
    parser.add_argument("--camoufox", action="store_true", help="Use isolated Camoufox observation for Botterdop")
    parser.add_argument("--scope-file", type=Path)
    parser.add_argument("--dry-run", action="store_true", help="Queue/triage selection without starting active testing")
    args = parser.parse_args()

    if args.resume:
        result = engine_run("", args.resume) if args.engine else resume(args.resume)
    elif not args.target.strip():
        result = queue_triage()
        if args.dry_run:
            print(json.dumps(result, indent=2))
            return 0
    else:
        result = engine_run(args.target.strip()) if args.engine else unified_run(args.target.strip(), args.scope_file, args.rate, args.scan, args.camoufox)

    print(json.dumps(result, indent=2))
    return 0 if result.get("ok") is not False else 2


if __name__ == "__main__":
    raise SystemExit(main())
