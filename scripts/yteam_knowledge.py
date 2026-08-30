#!/usr/bin/env python3
"""Durable cross-run knowledge base for Yteam.

Stores hypothesis verdicts, known bug signatures, and learned routing so later
runs can re-use what was verified or killed instead of re-testing the same
ground. This is the "learning loop" layer: it never turns a hypothesis into a
finding; it only records what has already been decided.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_KB = Path(os.environ.get("YTEAM_KB_DIR", ROOT / "runtime" / "knowledge" / "bug-signatures.jsonl"))
SECRET_RE = re.compile(r"(?i)(?:bearer\s+|session=|token=|password\s*[:=]|api[_-]?key\s*[:=])[^\s,;]+")


def now() -> str:
    return datetime.now(timezone.utc).isoformat()


def redact(value: object) -> object:
    if isinstance(value, str):
        return SECRET_RE.sub("<REDACTED>", value)
    if isinstance(value, dict):
        return {str(key): redact(item) for key, item in value.items() if str(key).lower() not in {"cookie", "authorization", "set-cookie"}}
    if isinstance(value, list):
        return [redact(item) for item in value]
    return value


def signature_for(target: str, endpoint: str, classes: list[str]) -> str:
    normalized = re.sub(r"[^a-zA-Z0-9]+", "_", f"{target}::{endpoint}::{','.join(sorted(classes))}").lower()
    return hashlib.sha256(normalized.encode("utf-8")).hexdigest()[:24]


def load(kb_path: Path | None = None) -> list[dict]:
    path = kb_path or DEFAULT_KB
    if not path.exists():
        return []
    records: list[dict] = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        if not line.strip():
            continue
        try:
            item = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(item, dict):
            records.append(item)
    return records


def append(record: dict, kb_path: Path | None = None) -> dict:
    path = kb_path or DEFAULT_KB
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(redact(record), sort_keys=True) + "\n")
    return record


def add_verdict(target: str, endpoint: str, classes: list[str], verdict: str, detail: str = "", kb_path: Path | None = None) -> dict:
    if verdict not in {"verified", "candidate", "killed", "blocked", "duplicate", "not_a_finding"}:
        raise ValueError(f"invalid verdict: {verdict}")
    record = {
        "kind": "verdict",
        "signature": signature_for(target, endpoint, classes),
        "target": target,
        "endpoint": endpoint,
        "classes": sorted(classes),
        "verdict": verdict,
        "detail": detail,
        "recorded_at": now(),
    }
    return append(record, kb_path)


def lookup(target: str, endpoint: str, classes: list[str], kb_path: Path | None = None) -> dict | None:
    signature = signature_for(target, endpoint, classes)
    for record in load(kb_path):
        if record.get("kind") == "verdict" and record.get("signature") == signature:
            return record
    return None


def dedupe_known_hypotheses(hypotheses: list[dict], kb_path: Path | None = None) -> dict:
    """Mark hypotheses that already have a learned verdict."""
    known = load(kb_path)
    by_sig: dict[str, dict] = {}
    for record in known:
        if record.get("kind") == "verdict":
            by_sig[record["signature"]] = record
    results = []
    for hypothesis in hypotheses:
        sig = signature_for(str(hypothesis.get("target", "")), str(hypothesis.get("key", "")).split("::")[-1], hypothesis.get("detected_classes", []) or hypothesis.get("suggested_tracks", []))
        learned = by_sig.get(sig)
        if learned:
            results.append({**hypothesis, "learned_verdict": learned.get("verdict"), "learned_at": learned.get("recorded_at")})
        else:
            results.append({**hypothesis, "learned_verdict": None})
    return {"checked": len(results), "known": sum(1 for item in results if item["learned_verdict"]), "hypotheses": results}


def stats(kb_path: Path | None = None) -> dict:
    records = load(kb_path)
    verdicts: dict[str, int] = {}
    for record in records:
        if record.get("kind") == "verdict":
            key = str(record.get("verdict"))
            verdicts[key] = verdicts.get(key, 0) + 1
    return {"total_records": len(records), "verdicts": verdicts}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    verdict = sub.add_parser("add", help="record a hypothesis verdict")
    verdict.add_argument("--target", required=True)
    verdict.add_argument("--endpoint", required=True)
    verdict.add_argument("--classes", nargs="*", default=[])
    verdict.add_argument("--verdict", required=True, choices=("verified", "candidate", "killed", "blocked", "duplicate", "not_a_finding"))
    verdict.add_argument("--detail", default="")
    lookup = sub.add_parser("lookup", help="lookup a learned verdict")
    lookup.add_argument("--target", required=True)
    lookup.add_argument("--endpoint", required=True)
    lookup.add_argument("--classes", nargs="*", default=[])
    stats_parser = sub.add_parser("stats")
    args = parser.parse_args()
    if args.command == "add":
        result = add_verdict(args.target, args.endpoint, args.classes, args.verdict, args.detail)
        print(json.dumps(result, indent=2))
    elif args.command == "lookup":
        result = lookup(args.target, args.endpoint, args.classes)
        print(json.dumps(result, indent=2) if result else "no learned verdict")
    else:
        print(json.dumps(stats(), indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
