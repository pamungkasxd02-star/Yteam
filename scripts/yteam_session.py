#!/usr/bin/env python3
"""Small native JSONL session store for YTEAM conversations."""

from __future__ import annotations

import json
import secrets
from datetime import datetime, timezone
from pathlib import Path


def now() -> str:
    return datetime.now(timezone.utc).isoformat()


class Session:
    def __init__(self, root: Path, session_id: str | None = None) -> None:
        self.root = root
        self.root.mkdir(parents=True, exist_ok=True)
        self.session_id = session_id or f"yteam_{secrets.token_urlsafe(12)}"
        self.path = self.root / f"{self.session_id}.jsonl"
        self.messages: list[dict[str, str]] = self._load()

    def _load(self) -> list[dict[str, str]]:
        if not self.path.exists():
            return []
        messages: list[dict[str, str]] = []
        for line in self.path.read_text(encoding="utf-8", errors="replace").splitlines():
            try:
                value = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(value, dict) and value.get("role") in {"user", "assistant", "system"}:
                messages.append({"role": str(value["role"]), "content": str(value.get("content", ""))})
        return messages

    def append(self, role: str, content: str) -> None:
        message = {"at": now(), "role": role, "content": content}
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(message, ensure_ascii=False) + "\n")
        self.messages.append({"role": role, "content": content})

    def conversation(self, limit: int = 40) -> list[dict[str, str]]:
        return self.messages[-limit:]
