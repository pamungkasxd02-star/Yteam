#!/usr/bin/env python3
"""Reviewed autonomous assessment workflow used by the durable worker.

This module binds the generic autonomy engine to YTEAM's existing safe scope,
recon, artifact-analysis, and triage components.  It deliberately exposes no
arbitrary shell tool and never auto-submits a report.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
for entry in (ROOT / "scripts", ROOT / "src"):
    if str(entry) not in sys.path:
        sys.path.insert(0, str(entry))


def _read_json(path: Path) -> dict[str, object]:
    try:
        value = json.loads(path.read_text(encoding="utf-8", errors="replace"))
    except (OSError, json.JSONDecodeError):
        return {}
    return value if isinstance(value, dict) else {}


def run(
    target: str,
    output: Path,
    pipeline_id: str,
    params: dict[str, object],
    store,
    aggregate_id: str,
) -> dict[str, object]:
    """Run the bounded autonomous workflow and return a serializable result."""
    from yteam_engine import Action, AutonomousAgent, Engine, ToolRegistry, ToolSpec

    output = Path(output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    engine = Engine(ROOT)
    registry = ToolRegistry(engine.policy, approval_store=store)

    def scope_validate(payload: dict[str, object]) -> dict[str, object]:
        from yteam_scope import validate

        scope_file = params.get("scope_file")
        decision = validate(
            str(payload["target"]),
            scope_file=Path(str(scope_file)) if scope_file else None,
        )
        if not decision.allowed:
            raise RuntimeError(f"scope blocked: {decision.reason}")
        return {"allowed": True, "mode": decision.mode, "reason": decision.reason}

    def deep_hunt(payload: dict[str, object]) -> dict[str, object]:
        from yteam_hunt import run as hunt

        manifest = hunt(
            str(payload["target"]),
            output,
            pipeline_id,
            int(params.get("depth", 2)),
            float(params.get("rate", 1.0)),
            bool(params.get("use_external", False)),
            bool(params.get("scan", False)),
            Path(str(params["scope_file"])) if params.get("scope_file") else None,
        )
        if manifest.get("status") == "blocked":
            raise RuntimeError("hunt pipeline was blocked by scope or policy")
        return {
            "status": manifest.get("status"),
            "stage_count": len(manifest.get("stages", [])),
            "tool_run_count": len(manifest.get("tool_runs", [])),
            "output": str(output),
        }

    def analyze_artifacts(_payload: dict[str, object]) -> dict[str, object]:
        hypotheses = _read_json(output / "hypotheses.json")
        tracks = _read_json(output / "track_plan.json")
        recon = _read_json(output / "recon.json")
        hypothesis_items = hypotheses.get("hypotheses", [])
        track_items = tracks.get("tracks", [])
        routes = recon.get("routes", recon.get("top_routes", []))
        return {
            "hypothesis_count": len(hypothesis_items) if isinstance(hypothesis_items, list) else 0,
            "eligible_tracks": [
                str(item.get("track"))
                for item in track_items
                if isinstance(item, dict) and item.get("status") == "eligible"
            ][:20] if isinstance(track_items, list) else [],
            "route_count": len(routes) if isinstance(routes, list) else 0,
            "signal": bool(hypothesis_items),
            "non_claim": "Hypotheses and recon signals are not vulnerability findings.",
        }

    def review_track(track: str):
        def handler(_payload: dict[str, object]) -> dict[str, object]:
            hypotheses = _read_json(output / "hypotheses.json").get("hypotheses", [])
            matched = [
                item for item in hypotheses
                if isinstance(item, dict) and track in (str(item.get("track", "")) + " " + str(item.get("class", ""))).lower()
            ] if isinstance(hypotheses, list) else []
            return {
                "track": track,
                "matched_hypotheses": len(matched),
                "signal": bool(matched),
                "reviewed_ids": [str(item.get("id", "")) for item in matched[:20]],
                "non_claim": "Artifact review prioritizes validation; it does not confirm a vulnerability.",
            }
        return handler

    def triage_readiness(payload: dict[str, object]) -> dict[str, object]:
        observations = dict(payload.get("context", {})).get("observations", {})
        analysis = observations.get("analyze", {}) if isinstance(observations, dict) else {}
        candidates = _read_json(output / "candidates.json")
        candidate_items = candidates.get("candidates", [])
        ready = [
            item for item in candidate_items
            if isinstance(item, dict) and item.get("status") in {"validated", "confirmed"}
        ] if isinstance(candidate_items, list) else []
        return {
            "validated_candidate_count": len(ready),
            "hypothesis_count": analysis.get("hypothesis_count", 0) if isinstance(analysis, dict) else 0,
            "report_ready": bool(ready),
            "objective_met": True,
            "next_action": "Review evidence and validate with researcher-owned fixtures; reports remain manual.",
            "auto_submit": False,
        }

    registry.register(ToolSpec("scope.validate", "Validate exact written authorization and target scope", "read", scope_validate, timeout_seconds=10))
    registry.register(ToolSpec("recon.deep_hunt", "Run bounded low-rate recon and evidence collection", "read", deep_hunt, timeout_seconds=300, max_output_bytes=128_000))
    registry.register(ToolSpec("artifact.analyze", "Analyze target-scoped recon artifacts into non-claim hypotheses", "read", analyze_artifacts, timeout_seconds=20))
    registry.register(ToolSpec("artifact.review.authorization", "Review authorization and object-boundary hypotheses", "read", review_track("authorization"), timeout_seconds=20))
    registry.register(ToolSpec("artifact.review.injection", "Review safe injection-canary hypotheses", "read", review_track("injection"), timeout_seconds=20))
    registry.register(ToolSpec("artifact.review.surface", "Review recon and application-surface hypotheses", "read", review_track("recon"), timeout_seconds=20))
    registry.register(ToolSpec("triage.readiness", "Apply evidence readiness gates without submitting a report", "read", triage_readiness, timeout_seconds=20))

    actions = [
        Action("scope", "scope.validate", objective="Confirm exact authorization before network access"),
        Action("recon", "recon.deep_hunt", depends_on=("scope",), objective="Build a bounded target surface inventory"),
        Action("analyze", "artifact.analyze", depends_on=("recon",), objective="Rank evidence-backed hypotheses"),
    ]

    def emit(kind: str, detail: str, payload: dict[str, object]) -> None:
        store.emit(aggregate_id, kind, detail, payload)

    def replan(agent_run, pending: list[Action]) -> list[Action]:
        known = {item.action_id for item in agent_run.results} | {item.id for item in pending}
        if "analyze" not in known or "triage" in known:
            return []
        analysis = next((item for item in agent_run.results if item.action_id == "analyze" and item.status == "completed"), None)
        if analysis is None:
            return []
        tracks = {str(item).lower() for item in analysis.observation.get("eligible_tracks", [])}
        review_specs = []
        if any("author" in item or "idor" in item or "access" in item for item in tracks):
            review_specs.append(("review-authorization", "artifact.review.authorization"))
        if any("inject" in item or "xss" in item or "sqli" in item for item in tracks):
            review_specs.append(("review-injection", "artifact.review.injection"))
        if tracks and not review_specs:
            review_specs.append(("review-surface", "artifact.review.surface"))
        reviews = [Action(action_id, tool, depends_on=("analyze",), objective=f"Review {tool.rsplit('.', 1)[-1]} evidence signals") for action_id, tool in review_specs]
        dependencies = tuple(item.id for item in reviews) or ("analyze",)
        reviews.append(Action("triage", "triage.readiness", depends_on=dependencies, objective="Determine whether any candidate is ready for manual reporting"))
        return reviews

    def save_checkpoint(checkpoint: dict[str, object]) -> None:
        store.save_agent_checkpoint(aggregate_id, target, str(checkpoint.get("status", "running")), checkpoint)

    existing = store.agent_run(aggregate_id)
    checkpoint = existing.get("checkpoint") if existing and existing.get("target") == target else None
    agent = AutonomousAgent(
        registry,
        max_rounds=12,
        max_actions=24,
        event_handler=emit,
        checkpoint_handler=save_checkpoint,
        cancel_handler=lambda: store.agent_cancel_requested(aggregate_id),
        replan_handler=replan,
    )
    result = agent.run(
        target,
        actions,
        context={"pipeline_id": pipeline_id, "output": str(output), "job_id": aggregate_id},
        checkpoint=checkpoint if isinstance(checkpoint, dict) else None,
    )
    summary = result.as_dict()
    (output / "autonomy.json").write_text(json.dumps(summary, indent=2, sort_keys=True), encoding="utf-8")
    return summary
