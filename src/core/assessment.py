"""Unified multi-pillar assessment runner."""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

from .platform import AssessmentContext, EngineRegistry, EngineResult, Policy, serialize_events


PILLAR_DAG: dict[str, tuple[str, ...]] = {
    "scope": (),
    "toolchain": ("scope",),
    "recon": ("scope", "toolchain"),
    "bot_bypass": ("recon",),
    "decrypt": ("recon",),
    "pentest_qa": ("recon",),
    "server_guard": ("recon",),
    "intelligence": ("recon", "bot_bypass", "decrypt", "pentest_qa", "server_guard"),
    "learning": ("intelligence",),
    "delivery": ("learning",),
}


def _run_python(script: str, args: list[str], cwd: Path, env: dict[str, str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run([sys.executable, str(cwd / "scripts" / script), *args], cwd=cwd, env=env, capture_output=True, text=True, errors="replace", check=False)


def build_registry() -> EngineRegistry:
    registry = EngineRegistry()
    project_root = Path(__file__).resolve().parents[2]

    def scope(ctx: AssessmentContext) -> EngineResult:
        ctx.require("recon")
        decision = ctx.state.get("scope") or {"target": ctx.target, "allowed": True, "mode": "operator-approved"}
        ctx.emit("scope.passed", "scope", target=ctx.target, mode=decision.get("mode", "unknown"))
        path = ctx.artifacts.json("scope.json", decision)
        return EngineResult("scope", "completed", "Scope gate recorded.", [str(path.relative_to(ctx.artifacts.root))])

    def toolchain(ctx: AssessmentContext) -> EngineResult:
        result = _run_python("yteam_toolchain.py", ["--json"], project_root, ctx.env)
        path = ctx.artifacts.text("toolchain.json", result.stdout)
        ctx.emit("toolchain.completed", "toolchain", returncode=result.returncode)
        return EngineResult("toolchain", "completed" if result.returncode == 0 else "degraded", "Toolchain inventory completed.", [str(path.relative_to(ctx.artifacts.root))])

    def recon(ctx: AssessmentContext) -> EngineResult:
        arguments = [
            "run",
            "--target",
            ctx.target,
            "--output",
            str(ctx.artifacts.root / "recon"),
            "--depth",
            "2",
            "--rate",
            str(ctx.policy.max_requests_per_second),
        ]
        if not ctx.policy.allow_external_tools:
            arguments.append("--no-external")
        if ctx.state.get("scan"):
            arguments.append("--scan")
        if ctx.state.get("scope_file"):
            arguments.extend(["--scope-file", str(ctx.state["scope_file"])])
        result = _run_python("yteam_hunt.py", arguments, project_root, ctx.env)
        ctx.artifacts.text("recon.stdout.txt", result.stdout)
        ctx.artifacts.text("recon.stderr.txt", result.stderr)
        ctx.emit("recon.completed" if result.returncode == 0 else "recon.blocked", "recon", returncode=result.returncode)
        return EngineResult("recon", "completed" if result.returncode == 0 else "blocked", "Full native YTEAM hunt engine completed." if result.returncode == 0 else "Recon blocked; inspect recon.stderr.txt.", ["recon/hunt.json", "recon/recon.json", "recon/routes.jsonl", "recon/track_plan.json", "recon/hidden_surface.json", "recon/yteam-skill-bundle.json"])

    def bot_bypass(ctx: AssessmentContext) -> EngineResult:
        from bot_bypass.detector import gate_summary

        recon = ctx.artifacts.path("recon/recon.json")
        data = json.loads(recon.read_text(encoding="utf-8")) if recon.exists() else {}
        observations = data.get("observations", []) if isinstance(data, dict) else []
        gates = [item.get("botterdop") or gate_summary(item.get("security_headers", {}), "", item.get("status")) for item in observations if isinstance(item, dict)]
        summary = data.get("botterdop", {}) if isinstance(data, dict) else {}
        camoufox = {"status": "not_requested"}
        if ctx.state.get("camoufox"):
            from bot_bypass.camoufox_adapter import CamoufoxConfig, run_camoufox

            camoufox = run_camoufox(CamoufoxConfig(ctx.target, ctx.artifacts.path("camoufox"), rate=ctx.policy.max_requests_per_second))
        result_artifacts = ["pillars/bot_bypass.json"]
        if ctx.state.get("camoufox"):
            result_artifacts.append("camoufox/camoufox.json")
        path = ctx.artifacts.json("pillars/bot_bypass.json", {"pillar": "bot_bypass", "gates": gates, "summary": summary, "camoufox": camoufox, "mode": "detect-classify-and-govern", "non_claim": "Gate detection is not a bypass or vulnerability proof."})
        return EngineResult("bot_bypass", "completed", "Bot/anti-automation response classification completed.", result_artifacts)

    def decrypt(ctx: AssessmentContext) -> EngineResult:
        from decrypt.detect import analyze_payload

        recon = ctx.artifacts.path("recon/recon.json")
        data = json.loads(recon.read_text(encoding="utf-8")) if recon.exists() else {}
        samples = []
        for item in data.get("observations", []) if isinstance(data, dict) else []:
            if isinstance(item, dict) and item.get("content_type"):
                samples.append(analyze_payload(str(item.get("content_type", ""))))
        path = ctx.artifacts.json("pillars/decrypt.json", {"pillar": "decrypt", "samples": samples[:32], "mode": "format-analysis", "non_claim": "Encoding/encryption heuristics require authorized client/source analysis and do not defeat cryptography."})
        return EngineResult("decrypt", "completed", "Response format analysis completed.", [str(path.relative_to(ctx.artifacts.root))])

    def pentest_qa(ctx: AssessmentContext) -> EngineResult:
        from pentest_qa.qa import build_matrix

        path = ctx.artifacts.json("pillars/pentest_qa.json", {"pillar": "pentest_qa", "matrix": build_matrix({}), "mode": "baseline-checklist"})
        return EngineResult("pentest_qa", "completed", "Pentest/QA baseline matrix created.", [str(path.relative_to(ctx.artifacts.root))])

    def server_guard(ctx: AssessmentContext) -> EngineResult:
        from server_guard.guard import build_guard_report

        headers: dict[str, str] = {}
        recon = ctx.artifacts.path("recon/recon.json")
        if recon.exists():
            data = json.loads(recon.read_text(encoding="utf-8"))
            observations = data.get("observations", []) if isinstance(data, dict) else []
            if observations and isinstance(observations[0], dict):
                headers = observations[0].get("security_headers", {})
        path = ctx.artifacts.json("pillars/server_guard.json", {"pillar": "server_guard", **build_guard_report(headers)})
        return EngineResult("server_guard", "completed", "Server hardening report created.", [str(path.relative_to(ctx.artifacts.root))])

    def intelligence(ctx: AssessmentContext) -> EngineResult:
        from pathlib import Path

        recon = ctx.artifacts.path("recon/intelligence/observations.jsonl")
        output = ctx.artifacts.path("pillars/hypotheses.json")
        if not recon.exists():
            ctx.artifacts.json("pillars/hypotheses.json", {"observation_count": 0, "hypothesis_count": 0, "hypotheses": []})
            return EngineResult("intelligence", "completed", "No observations available; empty hypothesis set recorded.", ["pillars/hypotheses.json"])
        result = _run_python("yteam_intelligence.py", ["analyze", "--ledger", str(recon), "--output", str(output)], project_root, ctx.env)
        ctx.emit("intelligence.completed", "intelligence", returncode=result.returncode)
        return EngineResult("intelligence", "completed" if result.returncode == 0 else "degraded", "Emerging-bug hypothesis analysis completed.", ["pillars/hypotheses.json"])

    def learning(ctx: AssessmentContext) -> EngineResult:
        path = ctx.artifacts.json("pillars/learning.json", {"pillar": "learning", "mode": "cross-run-suggestion", "source": "pillars/hypotheses.json", "next": "Record only verified/killed/blocked outcomes through yteam_knowledge.py; never save secrets or customer data."})
        return EngineResult("learning", "completed", "Learning handoff created.", [str(path.relative_to(ctx.artifacts.root))])

    def delivery(ctx: AssessmentContext) -> EngineResult:
        path = ctx.artifacts.json("assessment_summary.json", {"target": ctx.target, "run_id": ctx.run_id, "engines": list(PILLAR_DAG), "status": "requires_triage", "non_claim": "Assessment signals require human/LLM validation before reporting."})
        return EngineResult("delivery", "completed", "Unified assessment summary created; triage still required.", [str(path.relative_to(ctx.artifacts.root))])

    registry.register("scope", scope)
    registry.register("toolchain", toolchain)
    registry.register("recon", recon)
    registry.register("bot_bypass", bot_bypass)
    registry.register("decrypt", decrypt)
    registry.register("pentest_qa", pentest_qa)
    registry.register("server_guard", server_guard)
    registry.register("intelligence", intelligence)
    registry.register("learning", learning)
    registry.register("delivery", delivery)
    return registry


def ready_phases(state: dict[str, str]) -> list[str]:
    ready: list[str] = []
    for phase, dependencies in PILLAR_DAG.items():
        if state.get(phase) in {"completed", "blocked"}:
            continue
        if all(state.get(dependency) == "completed" for dependency in dependencies):
            ready.append(phase)
    return ready


def run_assessment(ctx: AssessmentContext, max_engines: int | None = None) -> dict[str, Any]:
    registry = build_registry()
    state: dict[str, str] = {phase: "pending" for phase in PILLAR_DAG}
    results: dict[str, dict[str, Any]] = {}
    executed = 0
    while True:
        ready = ready_phases(state)
        if not ready or (max_engines is not None and executed >= max_engines):
            break
        phase = ready[0]
        state[phase] = "active"
        ctx.emit("engine.started", phase, engine=phase)
        try:
            result = registry.get(phase)(ctx)
        except Exception as error:  # noqa: BLE001
            result = EngineResult(phase, "blocked", "Engine failed closed.", error=str(error))
        state[phase] = "completed" if result.status in {"completed", "degraded"} else "blocked"
        results[phase] = {"engine": result.engine, "status": result.status, "summary": result.summary, "artifacts": result.artifacts, "signals": result.signals, "next_actions": result.next_actions, "error": result.error}
        ctx.emit("engine.finished", phase, status=result.status, artifacts=result.artifacts)
        executed += 1
    manifest = {"schema_version": 1, "run_id": ctx.run_id, "target": ctx.target, "state": state, "results": results, "remaining": ready_phases(state), "events": serialize_events(ctx.events.events), "policy": ctx.policy.__dict__, "non_claim": "This is an assessment signal graph; it is not an automatic vulnerability verdict."}
    ctx.artifacts.json("assessment_manifest.json", manifest)
    return manifest
