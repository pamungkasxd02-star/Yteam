#!/usr/bin/env python3
"""Run one unified Yteam multi-pillar security assessment."""

from __future__ import annotations

import argparse
import json
import os
import secrets
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.parse import urlparse


ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
ASSESSMENTS_ROOT = Path(os.environ.get("YTEAM_ASSESSMENTS_ROOT", ROOT / "runtime" / "assessments"))
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))


def timestamp() -> str:
    return datetime.now(timezone.utc).isoformat()


def slugify(value: str) -> str:
    parsed = urlparse(value if "://" in value else f"https://{value}")
    raw = parsed.netloc or parsed.path or value
    return "".join(char.lower() if char.isalnum() else "_" for char in raw).strip("_")[:96] or "target"


def new_run_id() -> str:
    return f"asm_{datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%S')}_{secrets.token_hex(3)}"


def run(target: str, scope_file: Path | None, rate: float, no_external: bool, scan: bool, max_engines: int | None, camoufox: bool = False) -> dict:
    from core.assessment import PILLAR_DAG, run_assessment
    from core.platform import ArtifactStore, AssessmentContext, Policy
    from yteam_scope import validate

    normalized = target.strip()
    if not normalized:
        raise ValueError("target is required")
    if scope_file:
        scope_file = scope_file.expanduser().resolve()
    target_slug = slugify(normalized)
    decision = validate(normalized, target_slug, scope_file)
    run_id = new_run_id()
    artifact_root = ASSESSMENTS_ROOT / run_id / target_slug
    artifacts = ArtifactStore(artifact_root)
    policy = Policy(authorized=decision.allowed, read_only=True, max_requests_per_second=max(0.1, min(rate, 10.0)), allow_external_tools=not no_external)
    ctx = AssessmentContext(
        run_id=run_id,
        target=normalized,
        target_slug=target_slug,
        artifacts=artifacts,
        policy=policy,
        state={"scope": decision.__dict__, "scope_file": str(scope_file) if scope_file else "", "scan": scan, "camoufox": camoufox},
    )
    artifacts.json("scope.json", decision.__dict__)
    if not decision.allowed:
        result = {"schema_version": 1, "run_id": run_id, "target": normalized, "status": "blocked", "scope": decision.__dict__, "policy": policy.__dict__, "engines": list(PILLAR_DAG), "next": "Fix scope or provide an explicit authorized scope file."}
        artifacts.json("assessment_manifest.json", result)
        return {"ok": False, "run_id": run_id, "target": normalized, "status": "blocked", "phase": "scope", "scope": decision.__dict__, "run_dir": str(artifact_root), "context_path": str(artifact_root / "assessment_manifest.json")}

    result = run_assessment(ctx, max_engines=max_engines)
    result["scan_requested"] = scan
    result["scope"] = decision.__dict__
    result["finished_at"] = timestamp()
    artifacts.json("assessment_manifest.json", result)
    context = {
        "schema_version": 1,
        "run_id": run_id,
        "target": normalized,
        "target_slug": target_slug,
        "status": "ready_for_analysis",
        "policy": policy.__dict__,
        "scope": decision.__dict__,
        "camoufox_requested": camoufox,
        "engines": result["state"],
        "engine_results": result["results"],
        "remaining": result["remaining"],
        "required_next_step": "Read assessment_manifest.json and each pillar artifact. Select one evidence-backed hypothesis, then run the normal triage gate.",
        "non_claims": ["Pillar output is assessment evidence, not automatic vulnerability proof.", "A detected bot gate is not a bypass.", "A decrypt format heuristic is not cryptographic recovery.", "A missing hardening header is not automatically a bounty finding."],
    }
    artifacts.json("assessment_context.json", context)
    artifacts.path("assessment_context.md").write_text(
        "\n".join([
            "# Yteam Unified Assessment Context", "", f"- Target: `{normalized}`", f"- Run ID: `{run_id}`", f"- Scope: `{decision.mode}` — {decision.reason}", "", "## Engine state", "", *(f"- `{name}`: {state}" for name, state in result["state"].items()), "", "## Evidence contract", "", "1. Read `assessment_manifest.json` and pillar artifacts.", "2. Choose one concrete impact objective.", "3. Validate with safe, authorized, researcher-owned evidence.", "4. Record proof, negative result, or blocker.", "5. Run triage before creating a finding.", "", "## Non-claims", "", "- Pillar output is not automatic vulnerability proof.", "- Bot detection is not a bypass.", "- Encoding detection is not cryptographic recovery.", "- Missing headers alone are hardening notes, not bounty findings.", ""]),
        encoding="utf-8",
    )
    return {"ok": True, "run_id": run_id, "target": normalized, "target_slug": target_slug, "status": "ready_for_analysis", "phase": "triage", "run_dir": str(artifact_root), "context_path": str(artifact_root / "assessment_context.md"), "manifest_path": str(artifact_root / "assessment_manifest.json"), "engines": result["state"], "remaining": result["remaining"]}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target")
    parser.add_argument("--scope-file", type=Path)
    parser.add_argument("--rate", type=float, default=1.0)
    parser.add_argument("--no-external", action="store_true")
    parser.add_argument("--scan", action="store_true")
    parser.add_argument("--max-engines", type=int)
    parser.add_argument("--camoufox", action="store_true", help="Use isolated Camoufox browser observation for Botterdop")
    args = parser.parse_args()
    try:
        result = run(args.target, args.scope_file, args.rate, args.no_external, args.scan, args.max_engines, args.camoufox)
    except (ValueError, OSError, json.JSONDecodeError) as error:
        print(json.dumps({"ok": False, "error": str(error)}), file=sys.stderr)
        return 2
    print(json.dumps(result, indent=2))
    return 0 if result.get("ok") else 2


if __name__ == "__main__":
    raise SystemExit(main())
