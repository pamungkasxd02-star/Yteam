#!/usr/bin/env python3
"""Native YTEAM utilities for filtering, knowledge, secrets, and reports."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Iterable

from yteam_safety import redact_text, redact_value


ROOT = Path(__file__).resolve().parents[1]
RUNTIME = ROOT / "runtime"
SECRET_RE = re.compile(
    r"(?i)(bearer\s+|(?:api[_-]?key|secret|password|token|private[_-]?key)\s*[:=]\s*)([^\s,;\"']+)"
)
KNOWLEDGE = {
    "idor": "Compare the same object request under two researcher-owned identities and prove a foreign record or state change.",
    "bola": "Validate object-level authorization independently from route authentication; synthetic IDs and 401/403/404 differentials are not proof.",
    "ssrf": "Require an attributed OOB callback or internal response data; a reflected URL or DNS-only signal is not sufficient.",
    "cors": "Require attacker-origin reflection, credentials, a private response, and browser-readable proof before escalation.",
    "csrf": "Test only state-changing actions and require a reproducible victim-impact proof, not merely a missing token.",
    "xss": "Verify unescaped reflection and current-browser execution; alert-only behavior is not impact proof.",
    "sqli": "Establish a real data, timing, or boolean oracle and stop on cosmetic database errors.",
    "oauth": "Trace redirect URI, state, nonce, PKCE, and token exchange; redirect-only behavior is not account takeover.",
    "graphql": "Map query/mutation authorization and test cross-tenant IDs; introspection alone is informational.",
    "cloud": "Validate actual read/write permission on an in-scope resource without claiming or modifying cloud assets.",
}


def secret_scan(value: str) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    for match in SECRET_RE.finditer(value):
        findings.append({"kind": match.group(1).strip(" :="), "value": "<REDACTED>", "fingerprint": hashlib.sha256(match.group(2).encode()).hexdigest()[:12]})
    return findings


def filter_stream(lines: Iterable[str], limit: int = 40) -> list[str]:
    scored: list[tuple[int, str]] = []
    keywords = ("error", "token", "auth", "admin", "private", "internal", "secret", "redirect", "graphql", "status")
    for line in lines:
        clean = redact_text(line.rstrip())
        if not clean:
            continue
        score = sum(10 for keyword in keywords if keyword in clean.lower())
        if score:
            scored.append((score, clean))
    scored.sort(key=lambda item: (-item[0], item[1]))
    return [line for _, line in scored[: max(1, min(limit, 100))]]


def aggregate_reports(root: Path) -> dict[str, object]:
    findings: list[dict[str, object]] = []
    for path in sorted(root.glob("findings/*.md")):
        text = path.read_text(encoding="utf-8", errors="replace")
        severity = path.name.split("_", 1)[0].lower()
        findings.append({"file": str(path), "severity": severity, "title": next((line[2:].strip() for line in text.splitlines() if line.startswith("# ")), path.stem)})
    summary = {level: sum(1 for item in findings if item["severity"] == level) for level in ("critical", "high", "medium", "low", "informational")}
    result = {"schema_version": 1, "target": root.name, "finding_count": len(findings), "severity": summary, "findings": findings, "status": "unsubmitted"}
    (root / "metadata.json").write_text(json.dumps(redact_value(result), indent=2) + "\n", encoding="utf-8")
    (root / "SUMMARY.md").write_text("# YTEAM Summary\n\n" + "\n".join(f"- **{item['severity']}** — {item['title']} (`{item['file']}`)" for item in findings) + "\n", encoding="utf-8")
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    scan = sub.add_parser("secret-scan", aliases=["secret_scan"])
    scan.add_argument("path", nargs="?", type=Path)
    pipe = sub.add_parser("smart-pipe", aliases=["smart_pipe"])
    pipe.add_argument("--limit", type=int, default=40)
    knowledge = sub.add_parser("search-knowledge", aliases=["search_knowledge"])
    knowledge.add_argument("query", nargs="+")
    knowledge.add_argument("--limit", type=int, default=3)
    report = sub.add_parser("aggregate-reports", aliases=["aggregate_reports"])
    report.add_argument("target", type=Path)
    args = parser.parse_args()
    if args.command == "secret-scan":
        text = args.path.read_text(encoding="utf-8", errors="replace") if args.path else sys.stdin.read()
        print(json.dumps({"matches": secret_scan(text)}, indent=2))
    elif args.command == "smart-pipe":
        print("\n".join(filter_stream(sys.stdin, args.limit)))
    elif args.command == "search-knowledge":
        query = " ".join(args.query).lower()
        matches = [{"topic": topic, "content": content} for topic, content in KNOWLEDGE.items() if topic in query or any(word in content.lower() for word in query.split())]
        print(json.dumps({"query": query, "results": matches[: max(1, min(args.limit, 10))]}, indent=2))
    else:
        print(json.dumps(aggregate_reports(args.target), indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
