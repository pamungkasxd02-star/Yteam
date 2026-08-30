#!/usr/bin/env python3
"""Manage a small durable state ledger for autonomous Yteam bug-bounty runs."""

from __future__ import annotations

import argparse
import json
import os
import re
import secrets
import sys
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlparse

from yteam_safety import redact_value


ROOT = Path(__file__).resolve().parents[1]
RUNS = Path(os.environ.get("YTEAM_BB_RUNS", ROOT / "runtime" / "bb-runs"))
PHASES = ("scope", "recon", "mapping", "hypothesis", "validation", "triage", "delivery")
SECRET_PATTERN = re.compile(r"(?i)(bearer\s+|session=|token=|password\s*[:=]|api[_-]?key\s*[:=])[^\s,;]+")


def timestamp() -> str:
    return datetime.now(timezone.utc).isoformat()


def slugify(value: str) -> str:
    parsed = urlparse(value if "://" in value else f"https://{value}")
    base = parsed.netloc or parsed.path or value
    slug = re.sub(r"[^a-zA-Z0-9]+", "_", base).strip("_").lower()
    return slug[:96] or "target"


def clean(value: object) -> object:
    return redact_value(value)


def path_for(run_id: str) -> Path:
    if not re.fullmatch(r"[a-zA-Z0-9_-]{8,80}", run_id):
        raise ValueError("invalid run id")
    return RUNS / f"{run_id}.json"


def read(run_id: str) -> dict:
    path = path_for(run_id)
    if not path.exists():
        raise FileNotFoundError(f"run not found: {run_id}")
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("run ledger is not an object")
    return value


def write(run: dict) -> None:
    RUNS.mkdir(parents=True, exist_ok=True)
    path_for(str(run["run_id"])).write_text(json.dumps(clean(run), indent=2) + "\n", encoding="utf-8")


def prepare(target: str, run_id: str | None = None) -> dict:
    if not target.strip():
        raise ValueError("target is required for prepare; use /bb without arguments for queue triage")
    run_id = run_id or f"bb_{datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%S')}_{secrets.token_hex(3)}"
    run = {
        "schema_version": 1,
        "run_id": run_id,
        "target": target.strip(),
        "target_slug": slugify(target),
        "paths": {
            "recon": f"runtime/bb-runs/{run_id}/{slugify(target)}/recon",
            "evidence": f"runtime/bb-runs/{run_id}/{slugify(target)}/evidence",
            "reports": f"runtime/bb-runs/{run_id}/{slugify(target)}/reports",
            "intelligence": f"runtime/bb-runs/{run_id}/{slugify(target)}/intelligence",
        },
        "mode": "autonomous-deep-one",
        "status": "active",
        "current_phase": "scope",
        "created_at": timestamp(),
        "updated_at": timestamp(),
        "phases": {phase: {"status": "pending", "started_at": None, "completed_at": None} for phase in PHASES},
        "events": [],
        "hypothesis_count": 0,
        "non_claims": ["No vulnerability is confirmed by this ledger alone."],
    }
    run["phases"]["scope"]["status"] = "active"
    run["phases"]["scope"]["started_at"] = run["created_at"]
    write(run)
    return run


def event(run_id: str, kind: str, detail: str, phase: str | None) -> dict:
    run = read(run_id)
    if phase and phase not in PHASES:
        raise ValueError(f"unknown phase: {phase}")
    run["events"].append({"at": timestamp(), "kind": kind, "phase": phase or run["current_phase"], "detail": clean(detail)})
    if kind == "hypothesis":
        run["hypothesis_count"] = int(run.get("hypothesis_count", 0)) + 1
    run["updated_at"] = timestamp()
    write(run)
    return run


def advance(run_id: str, phase: str, status: str) -> dict:
    run = read(run_id)
    if phase not in PHASES:
        raise ValueError(f"unknown phase: {phase}")
    if status not in {"active", "completed", "blocked", "killed"}:
        raise ValueError("status must be active, completed, blocked, or killed")
    record = run["phases"][phase]
    record["status"] = status
    record.setdefault("started_at", timestamp())
    if status in {"completed", "blocked", "killed"}:
        record["completed_at"] = timestamp()
    run["current_phase"] = phase
    run["updated_at"] = timestamp()
    write(run)
    return run


def finish(run_id: str, status: str, detail: str) -> dict:
    if status not in {"pack", "cand", "mid", "blocked", "zero"}:
        raise ValueError("status must be pack, cand, mid, blocked, or zero")
    run = read(run_id)
    run["status"] = status
    run["updated_at"] = timestamp()
    run["events"].append({"at": run["updated_at"], "kind": "finish", "phase": run["current_phase"], "detail": clean(detail)})
    write(run)
    return run


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    prepare_parser = commands.add_parser("prepare")
    prepare_parser.add_argument("--target", required=True)
    event_parser = commands.add_parser("event")
    event_parser.add_argument("--run-id", required=True)
    event_parser.add_argument("--kind", required=True, choices=("note", "blocker", "negative", "hypothesis", "proof"))
    event_parser.add_argument("--detail", required=True)
    event_parser.add_argument("--phase")
    advance_parser = commands.add_parser("advance")
    advance_parser.add_argument("--run-id", required=True)
    advance_parser.add_argument("--phase", required=True, choices=PHASES)
    advance_parser.add_argument("--status", required=True, choices=("active", "completed", "blocked", "killed"))
    finish_parser = commands.add_parser("finish")
    finish_parser.add_argument("--run-id", required=True)
    finish_parser.add_argument("--status", required=True, choices=("pack", "cand", "mid", "blocked", "zero"))
    finish_parser.add_argument("--detail", required=True)
    status_parser = commands.add_parser("status")
    status_parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    try:
        if args.command == "prepare":
            result = prepare(args.target)
        elif args.command == "event":
            result = event(args.run_id, args.kind, args.detail, args.phase)
        elif args.command == "advance":
            result = advance(args.run_id, args.phase, args.status)
        elif args.command == "finish":
            result = finish(args.run_id, args.status, args.detail)
        else:
            result = read(args.run_id)
    except (FileNotFoundError, ValueError, json.JSONDecodeError) as error:
        print(f"bb_pipeline: {error}", file=sys.stderr)
        return 2
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
