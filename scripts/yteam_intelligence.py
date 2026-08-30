#!/usr/bin/env python3
"""Multi-dimensional emerging-bug intelligence engine for Yteam.

Turns redacted target observations into ranked *hypotheses* that the LLM can
route to the correct track/skill/model. It intentionally never writes a
finding; everything stays at `hypothesis` status until live proof and the
triage gate pass.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_DIR = Path(os.environ.get("YTEAM_INTEL_DIR", ROOT / "runtime" / "intelligence"))

KNOWN_CLASSES = {
    "idor": ("idor", "bola", "object-reference", "vertical-privesc"),
    "auth_bypass": ("auth_bypass", "auth-bypass", "sso", "saml", "mfa", "session", "token", "reset", "password"),
    "ato": ("ato", "account-takeover", "account_recovery", "password-reset", "oauth-link"),
    "sqli": ("sqli", "sql-injection", "database", "query"),
    "nosqli": ("nosqli", "nosql", "mongo", "operator-injection"),
    "xss": ("xss", "cross-site", "reflected", "stored", "dom", "mutation"),
    "ssrf": ("ssrf", "server-side-request", "url-fetch", "proxy", "webhook", "image-fetch"),
    "csrf": ("csrf", "cross-site-request", "state-changing", "same-site"),
    "cors": ("cors", "cross-origin", "access-control-allow-origin"),
    "oauth": ("oauth", "oidc", "openid", "redirect_uri", "authorization-code", "id-token"),
    "graphql": ("graphql", "gql", "introspection", "mutation", "query-cost"),
    "race": ("race", "concurrency", "double-spend", "toctou", "time-of-check"),
    "business_logic": ("business", "logic", "coupon", "refund", "payment", "order", "state-machine", "discount"),
    "upload": ("upload", "file-upload", "webshell", "attachment"),
    "lfi": ("lfi", "path-traversal", "directory-traversal", "file-include", "local-file"),
    "ssti": ("ssti", "template-injection", "jinja", "freemarker", "twig"),
    "rce": ("rce", "remote-code", "command-injection", "deserialization", "gadget"),
    "secret_exposure": ("secret", "key-leak", "credential", "env", "token-leak", "bucket"),
    "subdomain_takeover": ("subdomain-takeover", "dangling-cname", "claimable"),
    "api_misconfig": ("api-misconfig", "mass-assignment", "prototype-pollution", "http-method", "swagger", "openapi"),
    "cache_poisoning": ("cache-poisoning", "cache-key", "web-cache", "unkeyed"),
    "http_smuggling": ("smuggling", "http-smuggling", "cl.te", "te.cl"),
    "mfa_bypass": ("mfa", "2fa", "otp", "totp", "step-up"),
    "llm_ai": ("llm", "prompt-injection", "rag", "agent", "chatbot", "model"),
}

TRACK_BY_CLASS = {
    "authorization": {"idor", "bola", "api_misconfig", "cache_poisoning"},
    "authentication": {"auth_bypass", "ato", "oauth", "mfa_bypass"},
    "input-validation": {"sqli", "nosqli", "xss", "ssrf", "ssti", "xxe", "lfi", "upload", "rce", "http_smuggling"},
    "business-logic": {"race", "business_logic", "csrf", "graphql"},
    "cloud-and-infra": {"secret_exposure", "subdomain_takeover", "api_misconfig"},
    "client-and-browser": {"cors", "csrf", "xss", "oauth", "cache_poisoning"},
    "reporting": set(),
}

SECRET_RE = re.compile(r"(?i)(?:bearer\s+|session=|token=|password\s*[:=]|api[_-]?key\s*[:=])[^\s,;]+")
ENDPOINT_MARKER_RE = re.compile(r"[/:][^/\s]+")


def now() -> str:
    return datetime.now(timezone.utc).isoformat()


def redact(value: Any) -> Any:
    if isinstance(value, str):
        return SECRET_RE.sub("<REDACTED>", value)
    if isinstance(value, dict):
        return {str(key): redact(item) for key, item in value.items() if str(key).lower() not in {"cookie", "authorization", "set-cookie"}}
    if isinstance(value, list):
        return [redact(item) for item in value]
    return value


def _tokenize(value: str) -> set[str]:
    return {token.lower() for token in re.findall(r"[a-z0-9_-]+", value.lower()) if len(token) > 2}


def detect_classes(text: str) -> list[str]:
    haystack = _tokenize(text)
    found: list[str] = []
    for label, keywords in KNOWN_CLASSES.items():
        if any(keyword in haystack or keyword in text.lower() for keyword in keywords):
            found.append(label)
    return sorted(found)


def canonical_observation(raw: dict[str, Any]) -> dict[str, Any]:
    cleaned = redact(raw)
    endpoint = str(cleaned.get("endpoint", "")).strip()
    method = str(cleaned.get("method", "GET")).upper()
    status = cleaned.get("status")
    response_length = cleaned.get("response_length")
    shape = str(cleaned.get("response_shape", ""))
    body = str(cleaned.get("body", ""))[:2000]
    actor = str(cleaned.get("actor", "anonymous"))
    scope = str(cleaned.get("scope", "unknown"))
    resource = str(cleaned.get("resource", "")).strip()
    state = str(cleaned.get("state", "")).strip()
    prior_state = str(cleaned.get("prior_state", "")).strip()
    action = str(cleaned.get("action", "")).strip()
    source = str(cleaned.get("source", "manual"))
    tags = sorted({str(tag).lower() for tag in cleaned.get("tags", [])})
    fingerprint = hashlib.sha256(json.dumps({"method": method, "endpoint": endpoint, "status": status, "length": response_length, "shape": shape, "resource": resource, "actor": actor, "scope": scope, "state": state}, sort_keys=True).encode()).hexdigest()[:16]
    return {
        "kind": "observation",
        "observed_at": cleaned.get("observed_at") or now(),
        "target": str(cleaned.get("target", "")).strip(),
        "endpoint": endpoint,
        "method": method,
        "status": status,
        "response_length": response_length,
        "response_shape": shape,
        "body": body,
        "actor": actor,
        "scope": scope,
        "resource": resource,
        "state": state,
        "prior_state": prior_state,
        "action": action,
        "tags": tags,
        "source": source,
        "detected_classes": detect_classes(f"{endpoint} {shape} {body} {resource} {action} {' '.join(tags)}"),
        "fingerprint": fingerprint,
    }


def read_jsonl(path: Path) -> list[dict[str, Any]]:
    if not path.exists():
        return []
    records: list[dict[str, Any]] = []
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


def write_jsonl(path: Path, records: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(redact(item), sort_keys=True) + "\n" for item in records), encoding="utf-8")


def _group_key(item: dict[str, Any]) -> str:
    return f"{item.get('target')}::{item.get('method')}::{item.get('endpoint')}"


def analyze(records: list[dict[str, Any]]) -> dict[str, Any]:
    observations = [record for record in records if record.get("kind") == "observation"]
    grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for item in observations:
        grouped[_group_key(item)].append(item)

    known_class_histogram: Counter[str] = Counter()
    for item in observations:
        for label in item.get("detected_classes", []):
            known_class_histogram[label] += 1

    hypotheses: list[dict[str, Any]] = []
    for key, items in grouped.items():
        statuses = Counter(str(item.get("status")) for item in items)
        actors = {str(item.get("actor")) for item in items}
        scopes = {str(item.get("scope")) for item in items}
        resources = {str(item.get("resource")) for item in items if item.get("resource")}
        states = {str(item.get("state")) for item in items if item.get("state")}
        prior_states = {str(item.get("prior_state")) for item in items if item.get("prior_state")}
        actions = {str(item.get("action")) for item in items if item.get("action")}
        detected = {label for item in items for label in item.get("detected_classes", [])}
        tags = {tag for item in items for tag in item.get("tags", [])}
        fingerprints = {item.get("fingerprint") for item in items}

        signals: list[str] = []
        # 1. Cross-identity / cross-scope differential
        if len(actors) > 1:
            signals.append("actor differential")
        if len(scopes) > 1:
            signals.append("tenant/scope differential")
        if len(resources) > 1:
            signals.append("resource-boundary differential")
        # 2. Response differential
        if len(statuses) > 1:
            signals.append("status differential")
        if len({item.get("response_length") for item in items}) > 1:
            signals.append("response-length differential")
        # 3. State / sequence transition
        if states and prior_states and len({(item.get("prior_state"), item.get("state")) for item in items}) > 1:
            signals.append("state-transition differential")
        # 4. Behavioral change over repeated observations
        if len(fingerprints) > 1:
            signals.append("behavioral change across repeated observations")
        # 5. Unknown-class behavior
        unknown_class = not detected and not tags.intersection(KNOWN_CLASSES)
        if unknown_class:
            signals.append("no explicit known-class signature")
        # 6. Sensitive endpoint family
        endpoint_hint = _tokenize(str(items[0].get("endpoint", "")))
        if endpoint_hint & {"admin", "internal", "debug", "export", "webhook", "graphql", "payment", "invoice", "refund", "upload"}:
            signals.append("sensitive route family")

        if not signals:
            continue

        known_candidates = sorted(detected) if detected else []
        if not known_candidates and tags.intersection(KNOWN_CLASSES):
            known_candidates = sorted(tags.intersection(KNOWN_CLASSES))
        tracks = sorted({track for track, class_set in TRACK_BY_CLASS.items() if any(candidate in class_set for candidate in known_candidates)})
        if not tracks:
            tracks = ["web-surface"] if not known_candidates else ["reporting"]

        # 7. Novelty = how much this deviates from both known classes and its own baseline
        novelty = 5 + 12 * len(signals) + (18 if unknown_class else 0) + (8 if len(items) >= 3 else 0)
        novelty = min(100, novelty)
        # 8. Confidence = repeatability and signal strength
        confidence = 20 + 12 * len(signals) + (10 if len(items) >= 2 else 0) + (8 if len(fingerprints) > 1 else 0)
        confidence = min(95, confidence)

        safe_test_hint = "replay the smallest request with a clean baseline and researcher-owned identities; compare body, status, and authorization scope"
        if "state-transition differential" in signals:
            safe_test_hint = "verify whether the prior-state → current-state transition is reproducible and reachable only by the intended actor"
        if unknown_class:
            safe_test_hint = "capture a clean baseline, then isolate which input or identity change produced the deviation before labeling it"

        hypotheses.append({
            "kind": "emerging_bug_hypothesis",
            "status": "hypothesis",
            "created_at": now(),
            "key": key,
            "novelty_score": novelty,
            "confidence": confidence,
            "signals": signals,
            "unknown_class": unknown_class,
            "detected_classes": known_candidates,
            "suggested_tracks": tracks,
            "impact": "unknown — requires a scoped, non-destructive validation step",
            "next_safe_test": safe_test_hint,
            "observation_count": len(items),
            "unique_fingerprints": len(fingerprints),
            "actors": sorted(actors),
            "scopes": sorted(scopes),
            "resources": sorted(resources),
            "states": sorted(states),
            "provenance": [item.get("source", "manual") for item in items],
        })

    hypotheses.sort(key=lambda item: (item["novelty_score"], item["confidence"]), reverse=True)
    return {
        "generated_at": now(),
        "observation_count": len(observations),
        "hypothesis_count": len(hypotheses),
        "known_class_histogram": dict(known_class_histogram),
        "unknown_class_count": sum(1 for item in hypotheses if item["unknown_class"]),
        "hypotheses": hypotheses,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    record = sub.add_parser("record", help="append one redacted observation")
    record.add_argument("--input", required=True, help="JSON object, path to JSON, or '-' to read JSON from stdin")
    record.add_argument("--ledger", default=str(DEFAULT_DIR / "observations.jsonl"))
    inspect = sub.add_parser("analyze", help="analyze the observation ledger")
    inspect.add_argument("--ledger", default=str(DEFAULT_DIR / "observations.jsonl"))
    inspect.add_argument("--output", default=str(DEFAULT_DIR / "hypotheses.json"))
    args = parser.parse_args()
    if args.command == "record":
        if args.input == "-":
            raw = json.loads(sys.stdin.read())
        else:
            source = Path(args.input)
            raw = json.loads(source.read_text(encoding="utf-8")) if source.exists() else json.loads(args.input)
        ledger = Path(args.ledger)
        records = read_jsonl(ledger)
        records.append(canonical_observation(raw))
        write_jsonl(ledger, records)
        print(json.dumps(records[-1], indent=2))
        return 0
    result = analyze(read_jsonl(Path(args.ledger)))
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
