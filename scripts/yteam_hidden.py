#!/usr/bin/env python3
"""Bug-first hidden-surface analysis for Yteam.

This module turns already-collected, in-scope recon routes into a bounded
review plan. It does not probe targets, generate exploit payloads, or promote
an anomaly to a finding. Hermes may execute the returned checks only after the
normal scope and policy gates pass.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import defaultdict
from pathlib import Path
from urllib.parse import parse_qsl, urlparse


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from yteam_safety import redact_value, redact_url


ID_NAME_RE = re.compile(r"(?i)(^|[_-])(id|uid|uuid|guid|user|account|tenant|org|organization|workspace|project|team|order|invoice|document|file|message|thread|conversation|job|migration)([_-]|$)")
SENSITIVE_ROUTE_WORDS = {"admin", "internal", "debug", "export", "download", "billing", "invoice", "payment", "refund", "webhook", "token", "reset", "oauth", "graphql", "upload", "import"}
AUTH_WORDS = {"auth", "login", "signin", "sign-in", "session", "oauth", "sso", "mfa", "2fa", "otp", "token", "reset", "password", "verify"}
INPUT_WORDS = {"search", "query", "filter", "sort", "redirect", "callback", "url", "uri", "endpoint", "proxy", "fetch", "render", "preview", "template", "path", "file", "upload", "import", "webhook"}


def _route_path(url: str) -> str:
    parsed = urlparse(url)
    return parsed.path or "/"


def _segments(url: str) -> list[str]:
    return [part for part in _route_path(url).split("/") if part]


def _tokens(url: str) -> set[str]:
    return {token.lower() for token in re.findall(r"[a-zA-Z0-9_-]+", f"{_route_path(url)} {urlparse(url).query}") if len(token) > 1}


def _query_names(url: str) -> list[str]:
    return sorted({key for key, _ in parse_qsl(urlparse(url).query, keep_blank_values=True) if key})


def _id_names(url: str) -> list[str]:
    return sorted(name for name in _query_names(url) if ID_NAME_RE.search(name))


def _path_id_positions(url: str) -> list[int]:
    positions: list[int] = []
    for index, segment in enumerate(_segments(url)):
        if re.fullmatch(r"[0-9]+|[0-9a-fA-F]{8}-[0-9a-fA-F-]{20,}|[A-Za-z0-9_-]{16,}", segment):
            positions.append(index)
    return positions


def _family(url: str) -> str:
    parts = _segments(url)
    stable = [part for part in parts if not re.fullmatch(r"[0-9]+|[0-9a-fA-F]{8}-[0-9a-fA-F-]{20,}|[A-Za-z0-9_-]{16,}", part)]
    return "/" + "/".join(stable[:4])


def _safe_check(check_id: str, track: str, url: str, purpose: str, method: str = "GET", prerequisite: str = "exact in-scope target and a clean baseline") -> dict[str, object]:
    return {
        "check_id": check_id,
        "track": track,
        "method": method,
        "url": redact_url(url),
        "purpose": purpose,
        "prerequisite": prerequisite,
        "safe_action": "read-only comparison; do not access customer objects or perform a state-changing request",
        "success_signal": "repeatable security-boundary differential with concrete response data or an authorized test fixture",
        "stop_signal": "429, bot/WAF gate, unexpected side effect, missing scope, or missing researcher-owned fixture",
    }


def analyze_surface(target: str, recon: dict[str, object], max_hypotheses: int = 80) -> dict[str, object]:
    routes = [item for item in recon.get("routes", []) if isinstance(item, dict) and isinstance(item.get("url"), str)]
    routes = routes[:500]
    families: dict[str, list[dict[str, object]]] = defaultdict(list)
    for route in routes:
        families[_family(str(route["url"]))].append(route)

    route_map: list[dict[str, object]] = []
    hypotheses: list[dict[str, object]] = []
    seen: set[str] = set()

    for route in routes:
        url = str(route["url"])
        tokens = _tokens(url)
        query_names = _query_names(url)
        path_ids = _path_id_positions(url)
        route_id = str(route.get("route_id") or f"route-{len(route_map) + 1:04d}")
        flags: list[str] = []
        checks: list[dict[str, object]] = []
        score = int(route.get("priority") or 0)

        if path_ids or any(ID_NAME_RE.search(name) for name in query_names):
            flags.append("object-reference")
            score += 35
            check_id = f"authz-{route_id}"
            checks.append(_safe_check(check_id, "authorization", url, "Compare the same object reference under two researcher-owned identities or a synthetic nonexistent ID.", prerequisite="two researcher-owned identities or a designated synthetic fixture"))
            hypotheses.append({"id": check_id, "class": "idor_bola_candidate", "track": "authorization", "score": min(score, 100), "route": redact_url(url), "signal": "object reference in path/query", "next_safe_test": check_id})

        if tokens & SENSITIVE_ROUTE_WORDS:
            flags.append("sensitive-route-family")
            score += 20
        if tokens & AUTH_WORDS:
            flags.append("authentication-surface")
            score += 22
            check_id = f"auth-drift-{route_id}"
            checks.append(_safe_check(check_id, "authentication", url, "Compare authentication enforcement, redirect/state handling, and legacy/version siblings without credential guessing."))
            hypotheses.append({"id": check_id, "class": "authentication_boundary_drift", "track": "authentication", "score": min(score, 100), "route": redact_url(url), "signal": "authentication route family", "next_safe_test": check_id})

        if (tokens & INPUT_WORDS) or query_names:
            flags.append("user-controlled-input")
            score += 18
            check_id = f"input-{route_id}"
            checks.append(_safe_check(check_id, "input-validation", url, "Send one harmless canary and compare baseline response shape, encoding, and validation behavior."))
            hypotheses.append({"id": check_id, "class": "input_validation_candidate", "track": "input-validation", "score": min(score, 100), "route": redact_url(url), "parameters": query_names, "signal": "query or input-bearing route", "next_safe_test": check_id})

        if "graphql" in tokens or "gql" in tokens:
            flags.append("graphql-surface")
            score += 25
            check_id = f"graphql-{route_id}"
            checks.append(_safe_check(check_id, "authorization", url, "Map schema/operation behavior and compare resolver authorization with equivalent REST resources; introspection alone is not a finding." , method="POST", prerequisite="authorized GraphQL endpoint and researcher-owned fixture"))
            hypotheses.append({"id": check_id, "class": "graphql_rest_authorization_overlap", "track": "authorization", "score": min(score, 100), "route": redact_url(url), "signal": "GraphQL route", "next_safe_test": check_id})

        if tokens & {"webhook", "proxy", "fetch", "preview", "render", "callback", "redirect", "url", "uri"}:
            flags.append("server-side-url-or-browser-flow")
            score += 25
            check_id = f"url-flow-{route_id}"
            checks.append(_safe_check(check_id, "input-validation", url, "Verify whether the URL value is fetched or only reflected using a researcher-controlled callback/known external fixture; do not target internal metadata or private services.", prerequisite="approved callback endpoint or known public fixture"))
            hypotheses.append({"id": check_id, "class": "url_processing_boundary", "track": "input-validation", "score": min(score, 100), "route": redact_url(url), "parameters": query_names, "signal": "URL-processing or browser-flow marker", "next_safe_test": check_id})

        if tokens & {"order", "checkout", "payment", "coupon", "refund", "invite", "export", "billing"}:
            flags.append("business-state-family")
            score += 24
            check_id = f"state-{route_id}"
            checks.append(_safe_check(check_id, "business-logic", url, "Document the state machine and compare an allowed read-only transition against a synthetic/researcher-owned object; no financial or destructive action." , prerequisite="documented test tenant and explicit permission for a harmless fixture"))
            hypotheses.append({"id": check_id, "class": "business_state_transition", "track": "business-logic", "score": min(score, 100), "route": redact_url(url), "signal": "business-sensitive route family", "next_safe_test": check_id})

        versions = [part for part in _segments(url) if re.fullmatch(r"v[0-9]+", part, re.IGNORECASE)]
        if versions:
            flags.append("versioned-api")
            score += 14
            check_id = f"version-{route_id}"
            checks.append(_safe_check(check_id, "authorization", url, "Compare documented sibling API versions for authorization and response-field consistency using synthetic IDs."))
            hypotheses.append({"id": check_id, "class": "api_version_drift", "track": "authorization", "score": min(score, 100), "route": redact_url(url), "signal": "versioned API path", "next_safe_test": check_id})

        route_map.append({
            "route_id": route_id,
            "url": redact_url(url),
            "family": _family(url),
            "query_parameters": query_names,
            "path_id_positions": path_ids,
            "flags": sorted(set(flags)),
            "priority": min(score, 100),
            "sources": route.get("sources", []),
            "status": route.get("status"),
            "content_type": route.get("content_type", ""),
            "checks": checks,
        })

    # Sibling analysis finds missing/uneven coverage without actively probing.
    sibling_hypotheses: list[dict[str, object]] = []
    for family, members in families.items():
        if len(members) < 2:
            continue
        statuses = {item.get("status") for item in members}
        sources = {source for item in members for source in item.get("sources", []) if isinstance(source, str)}
        if len(statuses) > 1 or len(sources) > 1:
            urls = [redact_url(str(item["url"])) for item in members[:8]]
            key = f"sibling::{family}"
            if key not in seen:
                seen.add(key)
                sibling_hypotheses.append({
                    "id": f"sibling-{len(sibling_hypotheses) + 1:04d}",
                    "class": "sibling_endpoint_inconsistency",
                    "track": "authorization",
                    "score": 72,
                    "family": family,
                    "routes": urls,
                    "signal": "same route family has divergent status/source coverage",
                    "next_safe_test": "Replay only documented GETs with clean/synthetic fixtures and compare auth/error boundaries.",
                })

    hypotheses.extend(sibling_hypotheses)
    hypotheses.sort(key=lambda item: (-int(item.get("score", 0)), str(item.get("id", ""))))
    checks_by_id = {str(check["check_id"]): check for route in route_map for check in route.get("checks", [])}
    selected = hypotheses[: max(1, min(max_hypotheses, 120))]
    selected_ids = {str(item.get("next_safe_test")) for item in selected}
    checks = [check for check_id, check in checks_by_id.items() if check_id in selected_ids]
    return {
        "schema_version": 1,
        "engine": "yteam-hidden-surface",
        "target": target,
        "route_count": len(route_map),
        "family_count": len(families),
        "hypothesis_count": len(selected),
        "route_map": route_map,
        "hypotheses": selected,
        "safe_checks": checks,
        "non_claims": [
            "This is a route and trust-boundary review plan, not vulnerability proof.",
            "A sibling difference is not an authorization bypass without cross-identity evidence.",
            "A URL parameter is not SSRF without attributed server-side request evidence.",
            "A GraphQL endpoint or public schema is not a finding without concrete impact.",
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("recon", type=Path)
    parser.add_argument("--target", default="")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--limit", type=int, default=80)
    args = parser.parse_args()
    data = json.loads(args.recon.read_text(encoding="utf-8-sig"))
    result = analyze_surface(args.target or str(data.get("target", "")), data, args.limit)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(redact_value(result), indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"target": result["target"], "routes": result["route_count"], "hypotheses": result["hypothesis_count"], "output": str(args.output)}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
