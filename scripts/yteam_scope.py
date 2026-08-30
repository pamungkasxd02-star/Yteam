#!/usr/bin/env python3
"""Fail-closed scope helper for Yteam's autonomous stages."""

from __future__ import annotations

import fnmatch
import json
from dataclasses import asdict, dataclass
from pathlib import Path
from urllib.parse import urlparse


ROOT = Path(__file__).resolve().parents[1]


@dataclass(frozen=True)
class ScopeDecision:
    target: str
    allowed: bool
    mode: str
    scope_file: str
    matched_rule: str
    reason: str


def _read_rules(path: Path) -> tuple[list[str], list[str]]:
    try:
        import yaml

        data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
        if isinstance(data, dict):
            inside = data.get("in_scope", data.get("targets", []))
            outside = data.get("out_of_scope", [])
            return list(map(str, inside)) if isinstance(inside, list) else [], list(map(str, outside)) if isinstance(outside, list) else []
    except (ImportError, OSError, ValueError):
        pass
    return [], []


def _matches(target: str, rule: str) -> bool:
    value = target.lower()
    parsed = urlparse(value if "://" in value else f"https://{value}")
    host = (parsed.hostname or "").lower()
    rule = rule.strip().lower()
    if rule == "*":
        return True
    if rule.startswith("*."):
        return host == rule[2:] or host.endswith(rule[1:])
    return fnmatch.fnmatch(value, rule) or fnmatch.fnmatch(host, rule)


def find_scope(target_slug: str = "", explicit: Path | None = None) -> tuple[Path | None, list[str], list[str]]:
    candidates: list[Path] = []
    if explicit:
        candidates.append(explicit)
    if target_slug:
        candidates += [ROOT / "reports" / target_slug / "scope.yaml", ROOT.parent / "bugbounty_bugcrowd" / "programs" / target_slug / "scope.yaml"]
    candidates += [ROOT / "scope.yaml", ROOT.parent / "scope.yaml"]
    for path in candidates:
        if path.exists() and path.is_file():
            inside, outside = _read_rules(path)
            return path, inside, outside
    return None, [], []


def validate(target: str, target_slug: str = "", explicit: Path | None = None) -> ScopeDecision:
    path, inside, outside = find_scope(target_slug, explicit)
    for rule in outside:
        if _matches(target, rule):
            return ScopeDecision(target, False, "blocked", str(path or ""), rule, "Matched explicit out-of-scope rule.")
    if path is None:
        return ScopeDecision(target, True, "direct-target-only", "", "", "No scope file found; only the exact operator-supplied target is allowed.")
    for rule in inside:
        if _matches(target, rule):
            return ScopeDecision(target, True, "scoped", str(path), rule, "Matched explicit in-scope rule.")
    return ScopeDecision(target, False, "blocked", str(path), "", "Did not match any explicit in-scope rule.")


def main() -> int:
    import argparse

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target")
    parser.add_argument("--target-slug", default="")
    parser.add_argument("--scope-file", type=Path)
    args = parser.parse_args()
    decision = validate(args.target, args.target_slug, args.scope_file)
    print(json.dumps(asdict(decision), indent=2))
    return 0 if decision.allowed else 2


if __name__ == "__main__":
    raise SystemExit(main())
