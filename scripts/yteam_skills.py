#!/usr/bin/env python3
"""Build and query the native YTEAM skill registry and bundles."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCES = (ROOT / "skills",)
RISK_RULES = {
    "quarantined": ("reverse-shell", "webshell", "av-evasion", "credential-dump", "lateral-movement", "container-escape", "kernel-exploitation", "arbitrary-write-to-rce", "waf-bypass", "dependency-confusion", "ntlm-relay", "tunneling", "persistence"),
    "controlled": ("rce", "lfi", "ssrf", "ssti", "xxe", "deserialization", "upload", "cloud-iam", "jwt-crypto", "sqli", "nosql", "prototype-pollution"),
}
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


def source_roots() -> tuple[Path, ...]:
    """Return only first-party, repository-contained YTEAM skill roots."""
    return SOURCES


def risk_for(name: str, text: str) -> str:
    haystack = f"{name} {text[:5000]}".lower()
    if _any_word(haystack, RISK_RULES["quarantined"]):
        return "quarantined"
    if _any_word(haystack, RISK_RULES["controlled"]):
        return "controlled"
    return "safe_reference"


def _any_word(haystack: str, markers: tuple[str, ...]) -> bool:
    """Match markers as whole words so 'surfaces' doesn't trigger 'ssrf'."""
    for marker in markers:
        if re.search(rf"(?<![a-z0-9_-]){re.escape(marker)}(?![a-z0-9_-])", haystack):
            return True
    return False


def sections(text: str) -> list[dict[str, object]]:
    lines = text.splitlines()
    headings = [(index + 1, line.lstrip("#").strip()) for index, line in enumerate(lines) if line.startswith("#")]
    result: list[dict[str, object]] = []
    for index, (start, heading) in enumerate(headings):
        end = headings[index + 1][0] - 1 if index + 1 < len(headings) else len(lines)
        result.append({"heading": heading, "start_line": start, "end_line": end})
    return result


def _catalog_entries() -> list[dict[str, object]]:
    catalog_path = ROOT / "skills" / "catalog.json"
    if not catalog_path.exists():
        return []
    try:
        value = json.loads(catalog_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return []
    records = value.get("skills", []) if isinstance(value, dict) else []
    return [item for item in records if isinstance(item, dict)]


def registry() -> list[dict[str, object]]:
    entries: dict[str, dict[str, object]] = {}
    for source in source_roots():
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
                "path": str(path.relative_to(ROOT)).replace("\\", "/") if path.is_relative_to(ROOT) else str(path),
                "source": str(source.relative_to(ROOT)).replace("\\", "/") if source.is_relative_to(ROOT) else str(source),
                "categories": categories or ["general"],
                "content_sha256": hashlib.sha256(text.encode("utf-8")).hexdigest(),
                "size_bytes": len(text.encode("utf-8")),
                "line_count": len(text.splitlines()),
                "sections": sections(text),
                "risk": risk_for(name, text),
                "load_policy": "metadata_only" if risk_for(name, text) == "quarantined" else "on_demand",
            })
    for item in _catalog_entries():
        name = str(item.get("name", "")).strip()
        if not name:
            continue
        entries.setdefault(name, {
            "name": name,
            "description": str(item.get("description", "")),
            "path": "",
            "source": "yteam-built-in-catalog",
            "categories": list(item.get("categories", ["general"])),
            "content_sha256": str(item.get("content_sha256", "metadata-only")),
            "size_bytes": 0,
            "line_count": 0,
            "sections": list(item.get("sections", [])),
            "risk": str(item.get("risk", "controlled")),
            "load_policy": "metadata_only",
        })
    return sorted(entries.values(), key=lambda item: str(item["name"]))


def get_skill(items: list[dict[str, object]], skill_name: str, section: str = "", allow_controlled: bool = False) -> dict[str, object]:
    match = next((item for item in items if str(item["name"]).lower() == skill_name.strip().lower()), None)
    if not match:
        raise ValueError(f"skill not found: {skill_name}")
    path = Path(str(match["path"]))
    if not str(match.get("path", "")):
        return {**match, "content": "", "access": "metadata_only", "reason": "Built-in catalog entry. Add a reviewed first-party SKILL.md to enable body loading."}
    if not path.is_absolute():
        path = ROOT / path
    text = path.read_text(encoding="utf-8", errors="replace")
    if match.get("risk") == "quarantined" and not allow_controlled:
        return {**match, "content": "", "access": "quarantined", "reason": "Skill metadata is available; body loading is disabled for high-risk content."}
    if section:
        wanted = section.strip().lower()
        lines = text.splitlines()
        chosen = next((item for item in match.get("sections", []) if wanted in str(item.get("heading", "")).lower()), None)
        if chosen:
            text = "\n".join(lines[int(chosen["start_line"]) - 1:int(chosen["end_line"])])
    return {**match, "content": text[:80_000], "access": "loaded"}


def select_bundle(items: list[dict[str, object]], signals: list[str], limit: int = 18) -> list[dict[str, object]]:
    lowered = " ".join(signals).lower()
    scored: list[tuple[int, dict[str, object]]] = []
    for item in items:
        if item.get("risk") == "quarantined":
            continue
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
    parser.add_argument("command", choices=("index", "bundle", "get"))
    parser.add_argument("--output", type=Path, default=ROOT / "runtime" / "yteam-skill-registry.json")
    parser.add_argument("--signals", nargs="*", default=[])
    parser.add_argument("--limit", type=int, default=18)
    parser.add_argument("--skill")
    parser.add_argument("--section", default="")
    args = parser.parse_args()
    items = registry()
    if args.command == "index":
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps({"schema_version": 1, "skill_count": len(items), "skills": items}, indent=2) + "\n", encoding="utf-8")
        print(f"Indexed {len(items)} unique skills -> {args.output}")
        return 0
    if args.command == "get":
        print(json.dumps(get_skill(items, args.skill, args.section, False), indent=2))
        return 0
    bundle = select_bundle(items, args.signals, args.limit)
    print(json.dumps({"signals": args.signals, "selected_count": len(bundle), "skills": bundle, "non_claim": "Skill selection is guidance, not vulnerability proof."}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
