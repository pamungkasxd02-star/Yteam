"""Yteam Platform shared core.

Shared, scope-safe utilities used by all pillars (bot bypass, decrypt,
pentest/QA, server guard). Everything here is defense/authorized-testing
oriented and never performs destructive actions by itself.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
from datetime import datetime, timezone
from pathlib import Path

__version__ = "0.1.0"

SECRET_RE = re.compile(
    r"(?i)(?:bearer\s+|authorization\s*[:=]\s*(?:bearer\s+)?|cookie\s*[:=]\s*|"
    r"set-cookie\s*[:=]\s*|(?:access[_-]?token|refresh[_-]?token|id[_-]?token|csrf(?:[_-]?token)?|session(?:[_-]?id)?|"
    r"token|password|passwd|secret|api[_-]?key|private[_-]?key|client[_-]?secret|signature|sig)\s*[:=]\s*)"
    r"[^\s,;\"'}]+"
)


def redact(value: object) -> object:
    """Recursively redact credentials / tokens from any structure."""
    if isinstance(value, str):
        return SECRET_RE.sub("<REDACTED>", value)
    if isinstance(value, dict):
        def key_is_secret(key: object) -> bool:
            raw = str(key).replace("-", "_")
            normalized = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", raw).lower()
            return normalized in {"cookie", "authorization", "set_cookie"} or any(
                normalized == name or normalized.endswith(f"_{name}")
                for name in {"access_token", "refresh_token", "id_token", "csrf_token", "session", "session_id", "token", "password", "passwd", "secret", "api_key", "private_key", "client_secret", "signature", "sig"}
            )

        return {str(k): "<REDACTED>" if key_is_secret(k) else redact(v) for k, v in value.items()}
    if isinstance(value, list):
        return [redact(v) for v in value]
    return value


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def sha256_short(text: str, length: int = 16) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:length]


def write_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(redact(value), indent=2) + "\n", encoding="utf-8")


def load_json(path: Path) -> object:
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return None


def ensure_project_root() -> Path:
    configured = os.environ.get("YTEAM_PROJECT_ROOT")
    if configured:
        return Path(configured).resolve()
    return Path(__file__).resolve().parents[2]


from .platform import AssessmentContext, ArtifactStore, EngineRegistry, EngineResult, EventBus, Policy

__all__ = [
    "AssessmentContext",
    "ArtifactStore",
    "EngineRegistry",
    "EngineResult",
    "EventBus",
    "Policy",
    "redact",
    "now_iso",
    "sha256_short",
    "write_json",
    "load_json",
    "ensure_project_root",
]
