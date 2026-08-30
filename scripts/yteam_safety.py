#!/usr/bin/env python3
"""Shared redaction helpers for Yteam evidence and runtime artifacts."""

from __future__ import annotations

import re
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit


SENSITIVE_NAME_RE = re.compile(
    r"(?:^|[_-])(access[_-]?token|refresh[_-]?token|id[_-]?token|auth(?:orization)?|cookie|"
    r"set[_-]?cookie|csrf(?:[_-]?token)?|password|passwd|secret|api[_-]?key|private[_-]?key|"
    r"client[_-]?secret|signature|sig|nonce|state|code)(?:$|[_-])",
    re.IGNORECASE,
)
TEXT_SECRET_RE = re.compile(
    r'''(?i)(bearer\s+|authorization\s*[:=]\s*(?:bearer\s+)?|cookie\s*[:=]\s*|set-cookie\s*[:=]\s*|(?:access[_-]?token|refresh[_-]?token|id[_-]?token|csrf(?:[_-]?token)?|session(?:[_-]?id)?|token|password|passwd|secret|api[_-]?key|private[_-]?key|client[_-]?secret|signature|sig)\s*[:=]\s*)([^\s,;"}]+)'''
)
QUOTED_SECRET_RE = re.compile(
    r'''(?i)(["']?(?:access[_-]?token|refresh[_-]?token|id[_-]?token|csrf(?:[_-]?token)?|session(?:[_-]?id)?|token|password|passwd|secret|api[_-]?key|private[_-]?key|client[_-]?secret|signature|sig)["']?\s*[:=]\s*)(["']?)([^\s,;"}]+)(\2)'''
)
SENSITIVE_EXACT_NAMES = {
    "access_token", "refresh_token", "id_token", "session", "session_id", "token",
    "password", "passwd", "secret", "api_key", "private_key", "client_secret",
    "signature", "sig", "nonce", "state", "code", "csrf", "csrf_token",
    "authorization", "cookie", "set_cookie",
}


def sensitive_name(name: object) -> bool:
    raw = str(name).strip().replace(" ", "_")
    normalized = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", raw).lower()
    return normalized in SENSITIVE_EXACT_NAMES or bool(SENSITIVE_NAME_RE.search(normalized))


def redact_text(value: str) -> str:
    value = QUOTED_SECRET_RE.sub(lambda match: f"{match.group(1)}{match.group(2)}<REDACTED>{match.group(4)}", value)
    return TEXT_SECRET_RE.sub(lambda match: f"{match.group(1)}<REDACTED>", value)


def redact_url(value: str) -> str:
    try:
        parsed = urlsplit(value)
    except ValueError:
        return redact_text(value)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return redact_text(value)
    query = [(key, "<REDACTED>" if sensitive_name(key) else item) for key, item in parse_qsl(parsed.query, keep_blank_values=True)]
    fragment = parsed.fragment
    fragment_pairs = parse_qsl(fragment, keep_blank_values=True)
    if fragment_pairs:
        fragment = urlencode([(key, "<REDACTED>" if sensitive_name(key) else item) for key, item in fragment_pairs])
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path, urlencode(query), fragment))


def redact_value(value: object) -> object:
    if isinstance(value, dict):
        return {str(key): "<REDACTED>" if sensitive_name(key) else redact_value(item) for key, item in value.items()}
    if isinstance(value, list):
        return [redact_value(item) for item in value]
    if isinstance(value, tuple):
        return [redact_value(item) for item in value]
    if isinstance(value, str):
        return redact_text(value)
    return value
