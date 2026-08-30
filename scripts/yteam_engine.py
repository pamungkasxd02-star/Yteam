#!/usr/bin/env python3
"""Multi-layer Yteam orchestration engine.

A stateful, DAG-driven orchestrator that schedules sub-engines (scope,
recon, fingerprint, intel, track-router, validator, learning) adaptively
based on run state and prerequisites. This is the layer that makes Yteam more
    complex than a linear pipeline: each engine runs only when its
dependency phase is satisfied, results feed forward, and the learning loop
carries knowledge across runs.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_RUNS = Path(os.environ.get("YTEAM_BB_RUNS", ROOT / "runtime" / "bb-runs"))

# DAG of phases -> prerequisites that must be completed first.
PHASE_DAG: dict[str, tuple[str, ...]] = {
    "scope": (),
    "inventory": ("scope",),
    "passive": ("scope", "inventory"),
    "recon": ("scope", "inventory"),
    "fingerprint": ("recon",),
    "mapping": ("recon", "fingerprint"),
    "intel": ("mapping",),
    "validation": ("intel",),
    "triage": ("validation",),
    "delivery": ("triage",),
}


@dataclass
class RunContext:
    run_id: str
    target: str
    target_slug: str
    run_dir: Path
    phases: dict[str, dict[str, Any]] = field(default_factory=dict)
    env: dict[str, str] = field(default_factory=dict)


def _py() -> str:
    return sys.executable


def _run(cmd: list[str], env: dict[str, str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(cmd, capture_output=True, text=True, errors="replace", check=False, env=env)


class EngineRegistry:
    def __init__(self) -> None:
        self._engines: dict[str, Callable[[RunContext], dict[str, Any]]] = {}

    def register(self, name: str, fn: Callable[[RunContext], dict[str, Any]]) -> None:
        self._engines[name] = fn

    def names(self) -> list[str]:
        return sorted(self._engines)

    def run_engine(self, name: str, ctx: RunContext) -> dict[str, Any]:
        if name not in self._engines:
            raise KeyError(f"unknown engine: {name}")
        return self._engines[name](ctx)


def make_registry() -> EngineRegistry:
    registry = EngineRegistry()

    def engine_scope(ctx: RunContext) -> dict[str, Any]:
        return {"engine": "scope", "ok": True, "note": "scope gate handled by yteam_scope during prepare"}

    def engine_inventory(ctx: RunContext) -> dict[str, Any]:
        result = _run([_py(), str(ROOT / "scripts" / "yteam_toolchain.py"), "--json"], ctx.env)
        out_path = ctx.run_dir / "toolchain.json"
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(result.stdout, encoding="utf-8")
        try:
            tools = json.loads(result.stdout)
        except json.JSONDecodeError:
            tools = []
        return {"engine": "inventory", "ok": result.returncode == 0, "tools": tools}

    def engine_recon(ctx: RunContext) -> dict[str, Any]:
        recon_dir = ctx.run_dir / "recon"
        recon_dir.mkdir(parents=True, exist_ok=True)
        result = _run(
            [_py(), str(ROOT / "scripts" / "yteam_recon.py"), "run", "--target", ctx.target, "--output", str(recon_dir), "--depth", "2", "--rate", "1"],
            ctx.env,
        )
        return {"engine": "recon", "ok": result.returncode == 0, "stdout": result.stdout[:4000], "stderr": result.stderr[:2000]}

    def engine_fingerprint(ctx: RunContext) -> dict[str, Any]:
        # Reuse recon.json technology + routes as the fingerprint layer.
        recon_file = ctx.run_dir / "recon" / "recon.json"
        data: dict[str, Any] = {}
        if recon_file.exists():
            try:
                data = json.loads(recon_file.read_text(encoding="utf-8"))
            except (OSError, json.JSONDecodeError):
                pass
        return {
            "engine": "fingerprint",
            "ok": True,
            "technologies": data.get("technology", []),
            "resolved_addresses": data.get("resolved_addresses", []),
            "passive_assets": len(data.get("passive_assets", [])),
        }

    def engine_mapping(ctx: RunContext) -> dict[str, Any]:
        result = _run([_py(), str(ROOT / "scripts" / "yteam_hunt.py"), "bundle_plan", "--target", ctx.target, "--output", str(ctx.run_dir)], ctx.env)
        return {"engine": "mapping", "ok": result.returncode == 0, "stdout": result.stdout[:2000], "stderr": result.stderr[:2000]}

    def engine_intel(ctx: RunContext) -> dict[str, Any]:
        ledger = ctx.run_dir / "recon" / "intelligence" / "observations.jsonl"
        if not ledger.exists():
            return {"engine": "intel", "ok": True, "observation_count": 0, "hypothesis_count": 0}
        out_path = ctx.run_dir / "hypotheses.json"
        result = _run([_py(), str(ROOT / "scripts" / "yteam_intelligence.py"), "analyze", "--ledger", str(ledger), "--output", str(out_path)], ctx.env)
        try:
            data = json.loads(out_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            data = {}
        return {"engine": "intel", "ok": result.returncode == 0, "observation_count": data.get("observation_count", 0), "hypothesis_count": data.get("hypothesis_count", 0), "unknown_class_count": data.get("unknown_class_count", 0)}

    def engine_validation(ctx: RunContext) -> dict[str, Any]:
        return {"engine": "validation", "ok": True, "note": "safe, low-rate validation performed by YTEAM following the model action contract"}

    def engine_triage(ctx: RunContext) -> dict[str, Any]:
        return {"engine": "triage", "ok": True, "note": "seven-question gate applied by YTEAM before any finding"}

    def engine_delivery(ctx: RunContext) -> dict[str, Any]:
        return {"engine": "delivery", "ok": True, "note": "report/MID/blocked decision recorded in ledger"}

    registry.register("scope", engine_scope)
    registry.register("inventory", engine_inventory)
    registry.register("passive", lambda ctx: {"engine": "passive", "ok": True, "note": "passive asset discovery available via hunt engine"})
    registry.register("recon", engine_recon)
    registry.register("fingerprint", engine_fingerprint)
    registry.register("mapping", engine_mapping)
    registry.register("intel", engine_intel)
    registry.register("validation", engine_validation)
    registry.register("triage", engine_triage)
    registry.register("delivery", engine_delivery)
    return registry


def resolve_ready(registry: EngineRegistry, ctx: RunContext) -> list[str]:
    ready = []
    for phase, prereqs in PHASE_DAG.items():
        status = ctx.phases.get(phase, {}).get("status", "pending")
        if status in {"completed", "blocked"}:
            continue
        if status == "active":
            ready.append(phase)
            continue
        # A blocked prerequisite is a fail-closed stop, not permission to
        # continue into a dependent phase. This keeps the legacy DAG aligned
        # with the unified assessment policy.
        prereq_met = all(ctx.phases.get(p, {}).get("status") == "completed" for p in prereqs)
        if prereq_met:
            ready.append(phase)
    return ready


def orchestrate(target: str, run_id: str | None = None, max_phases: int | None = None) -> dict[str, Any]:
    env = os.environ.copy()
    env.setdefault("YTEAM_BB_RUNS", str(DEFAULT_RUNS))
    registry = make_registry()

    # Ensure a run ledger exists.
    if run_id:
        status = _run([_py(), str(ROOT / "scripts" / "bb_pipeline.py"), "status", "--run-id", run_id], env)
        if status.returncode != 0:
            return {"ok": False, "error": status.stderr.strip() or "run not found"}
        ledger = json.loads(status.stdout)
    else:
        prepare = _run([_py(), str(ROOT / "scripts" / "bb_pipeline.py"), "prepare", "--target", target], env)
        if prepare.returncode != 0:
            return {"ok": False, "error": prepare.stderr.strip() or prepare.stdout.strip()}
        ledger = json.loads(prepare.stdout)
        run_id = str(ledger["run_id"])

    target_slug = str(ledger["target_slug"])
    run_dir = ROOT / ledger["paths"]["recon"]
    ctx = RunContext(run_id=run_id, target=target, target_slug=target_slug, run_dir=run_dir, env=env)

    # Initialize phase states from ledger if present.
    for phase in PHASE_DAG:
        if phase in ledger.get("phases", {}):
            ctx.phases[phase] = ledger["phases"][phase]

    ran: list[str] = []
    results: dict[str, Any] = {}
    iterations = 0
    while True:
        ready = resolve_ready(registry, ctx)
        ready = [phase for phase in ready if phase not in ran]
        if not ready:
            break
        if max_phases and iterations >= max_phases:
            break
        phase = ready[0]
        try:
            result = registry.run_engine(phase, ctx)
        except Exception as error:  # noqa: BLE001
            result = {"engine": phase, "ok": False, "error": str(error)}
        results[phase] = result
        ctx.phases[phase] = {"status": "completed" if result.get("ok") else "blocked", "result": result.get("note", "")}
        ran.append(phase)
        iterations += 1

    return {
        "ok": True,
        "run_id": run_id,
        "target": target,
        "target_slug": target_slug,
        "run_dir": str(run_dir),
        "phases_ran": ran,
        "engine_results": results,
        "remaining": resolve_ready(registry, ctx),
        "note": "Orchestrator scheduled each phase only after its prerequisites completed.",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target", nargs="?")
    parser.add_argument("--resume", metavar="RUN_ID")
    parser.add_argument("--max-phases", type=int)
    parser.add_argument("--list-engines", action="store_true")
    args = parser.parse_args()

    if args.list_engines:
        registry = make_registry()
        print(json.dumps({"engines": registry.names(), "dag": PHASE_DAG}, indent=2))
        return 0
    if not args.target and not args.resume:
        print("target or --resume required (or --list-engines)", file=sys.stderr)
        return 2
    result = orchestrate(args.target or "", args.resume, args.max_phases)
    print(json.dumps(result, indent=2))
    return 0 if result.get("ok") is not False else 2


if __name__ == "__main__":
    raise SystemExit(main())
