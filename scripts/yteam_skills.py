#!/usr/bin/env python3
"""Build and query the native YTEAM skill registry and bundles."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCES = (
    ROOT / "skills",
)
KEYWORDS = {
    "recon": ("recon", "scope", "subdomain", "api", "web2", "enumeration", "osint", "source"),
    "authorization": ("idor", "bola", "authorization", "authz", "access", "tenant", "permission"),
    "authentication": ("authentication", "authbypass", "jwt", "oauth", "saml", "session", "mfa", "captcha", "ato"),
    "injection": ("sqli", "sql", "xss", "ssrf", "ssti", "xxe", "injection", "lfi", "upload", "prototype", "ldap", "deserialization"),
    "logic": ("business", "race", "logic", "csrf", "coupon", "payment", "refund", "graphql"),
    "infra": ("cloud", "k8s", "kubernetes", "cicd", "subdomain", "network", "vcenter", "windows", "linux", "supply"),
    "client": ("browser", "cors", "dom", "clickjacking", "websocket", "mobile", "android", "api"),
    "reporting": ("report", "triage", "evidence", "bugcrowd", "bounty", "validation"),
}


def value(text: str, key: str) -> str:
    match = re.search(rf"^\s*{re.escape(key)}:\s*(.*?)\s*$", text, re.MULTILINE)
    return match.group(1).strip().strip("\"'") if match else ""


def registry() -> list[dict[str, object]]:
    entries: dict[str, dict[str, object]] = {}
    for source in SOURCES:
        if not source.exists():
            continue
        for path in source.rglob("SKILL.md"):
            text = path.read_text(encoding="utf-8", errors="replace")
            name = value(text, "name") or path.parent.name
            description = value(text, "description")
            body = f"{name} {description} {path.parent}".lower()
            categories = sorted(category for category, words in KEYWORDS.items() if any(word in body for word in words))
            entries.setdefault(name, {
                "name": name,
                "description": description,
                "path": str(path.relative_to(ROOT)).replace("\\", "/"),
                "source": str(source.relative_to(ROOT)).replace("\\", "/"),
                "categories": categories or ["general"],
                "content_sha256": hashlib.sha256(text.encode("utf-8")).hexdigest(),
            })
    return sorted(entries.values(), key=lambda item: str(item["name"]))


def select_bundle(items: list[dict[str, object]], signals: list[str], limit: int = 18) -> list[dict[str, object]]:
    lowered = " ".join(signals).lower()
    scored: list[tuple[int, dict[str, object]]] = []
    for item in items:
        haystack = " ".join([str(item["name"]), str(item["description"]), *[str(category) for category in item["categories"]]]).lower()
        score = sum(10 for keyword in re.findall(r"[a-z0-9_-]+", lowered) if keyword and keyword in haystack)
        if "recon" in lowered and "recon" in haystack:
            score += 30
        if any(token in lowered for token in ("api", "graphql", "admin", "auth", "login")) and any(token in haystack for token in ("idor", "bola", "authorization", "auth")):
            score += 25
        if score:
            scored.append((score, item))
    scored.sort(key=lambda pair: (-pair[0], str(pair[1]["name"])))
    return [{**item, "bundle_score": score} for score, item in scored[:max(1, min(limit, 32))]]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("index", "bundle"))
    parser.add_argument("--output", type=Path, default=ROOT / "runtime" / "yteam-skill-registry.json")
    parser.add_argument("--signals", nargs="*", default=[])
    parser.add_argument("--limit", type=int, default=18)
    args = parser.parse_args()
    items = registry()
    if args.command == "index":
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps({"schema_version": 1, "skill_count": len(items), "skills": items}, indent=2) + "\n", encoding="utf-8")
        print(f"Indexed {len(items)} unique skills -> {args.output}")
        return 0
    bundle = select_bundle(items, args.signals, args.limit)
    print(json.dumps({"signals": args.signals, "selected_count": len(bundle), "skills": bundle, "non_claim": "Skill selection is guidance, not vulnerability proof."}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
