#!/usr/bin/env python3
"""Safe durable learning memory for YTEAM.

Memory is deliberately two-phase: observations become proposals first and only
explicitly verified, redacted lessons are injected into future model context.
This prevents an AI-generated guess from silently becoming policy or truth.
"""

from __future__ import annotations

import hashlib
import json
import re
import secrets
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable

from yteam_safety import redact_text, redact_value


SECRET_WORDS = re.compile(r"(?i)\b(?:password|passwd|secret|token|cookie|authorization|bearer|api[_-]?key|private[_-]?key)\b")


def now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _safe(value: object) -> object:
    return redact_value(value)


class LearningMemory:
    """Append-only JSONL memory with verified-context retrieval."""

    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)

    def _records(self) -> list[dict[str, object]]:
        if not self.path.exists():
            return []
        records: list[dict[str, object]] = []
        for line in self.path.read_text(encoding="utf-8", errors="replace").splitlines():
            try:
                value = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(value, dict):
                records.append(value)
        return records

    def _append(self, value: dict[str, object]) -> dict[str, object]:
        safe = _safe(value)
        assert isinstance(safe, dict)
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(safe, ensure_ascii=False, sort_keys=True) + "\n")
        return safe

    def propose(self, text: str, source: str = "operator", tags: Iterable[str] = ()) -> dict[str, object]:
        clean = redact_text(text).strip()
        if not clean or SECRET_WORDS.search(clean):
            raise ValueError("memory proposal is empty or contains a sensitive field name")
        proposal_id = "lp_" + secrets.token_hex(8)
        fingerprint = hashlib.sha256(clean.lower().encode("utf-8")).hexdigest()[:16]
        return self._append({
            "kind": "learning_proposal",
            "id": proposal_id,
            "fingerprint": fingerprint,
            "text": clean[:1200],
            "source": source[:80],
            "tags": sorted({str(tag)[:40] for tag in tags}),
            "status": "proposed",
            "recorded_at": now(),
        })

    def verify(self, proposal_id: str, verifier: str = "operator") -> dict[str, object]:
        proposals = [item for item in self._records() if item.get("kind") == "learning_proposal" and item.get("id") == proposal_id]
        if not proposals:
            raise ValueError(f"unknown learning proposal: {proposal_id}")
        proposal = proposals[-1]
        return self._append({
            "kind": "learning_lesson",
            "id": proposal_id,
            "fingerprint": proposal.get("fingerprint", ""),
            "text": proposal.get("text", ""),
            "tags": proposal.get("tags", []),
            "status": "verified",
            "verified_by": verifier[:80],
            "verified_at": now(),
        })

    def verified(self, query: str = "", limit: int = 12) -> list[dict[str, object]]:
        latest: dict[str, dict[str, object]] = {}
        for item in self._records():
            if item.get("kind") == "learning_lesson" and item.get("status") == "verified":
                latest[str(item.get("id"))] = item
        words = [word for word in re.findall(r"[a-z0-9_-]+", query.lower()) if len(word) > 2]
        values = list(latest.values())
        if words:
            values = [item for item in values if any(word in str(item.get("text", "")).lower() or word in json.dumps(item.get("tags", [])).lower() for word in words)]
        return values[-max(1, min(limit, 40)):]

    def proposals(self, limit: int = 12) -> list[dict[str, object]]:
        records = [item for item in self._records() if item.get("kind") == "learning_proposal" and item.get("status") == "proposed"]
        return records[-max(1, min(limit, 40)):]

    def context(self, query: str = "", limit: int = 8) -> str:
        lessons = self.verified(query, limit)
        if not lessons:
            return "No verified YTEAM lessons are available for this request."
        return "\n".join(f"- {item.get('text', '')}" for item in lessons)

    def summary(self) -> dict[str, int]:
        records = self._records()
        return {
            "records": len(records),
            "proposals": sum(1 for item in records if item.get("kind") == "learning_proposal" and item.get("status") == "proposed"),
            "verified": len({str(item.get("id")) for item in records if item.get("kind") == "learning_lesson" and item.get("status") == "verified"}),
        }
