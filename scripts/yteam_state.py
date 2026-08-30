#!/usr/bin/env python3
"""Small durable state layer for YTEAM sessions and replayable events.

This module intentionally uses only Python's standard-library sqlite3 module.
It provides the narrow waist between frontends, remote adapters, and the agent
turn loop without importing a third-party runtime.
"""

from __future__ import annotations

import json
import sqlite3
import threading
import time
from contextlib import contextmanager
from pathlib import Path

from yteam_safety import redact_text, redact_value


SCHEMA = """
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    created_at REAL NOT NULL,
    updated_at REAL NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    metadata TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id, id);
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aggregate_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    kind TEXT NOT NULL,
    detail TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    created_at REAL NOT NULL,
    UNIQUE(aggregate_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_events_aggregate ON events(aggregate_id, sequence);
"""


class StateStore:
    """Thread-safe SQLite store with ordered messages and aggregate events."""

    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._lock = threading.RLock()
        with self._connection() as connection:
            connection.executescript(SCHEMA)

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self.path, timeout=15, check_same_thread=False)
        connection.row_factory = sqlite3.Row
        connection.execute("PRAGMA busy_timeout=15000")
        return connection

    @contextmanager
    def _connection(self):
        connection = self._connect()
        try:
            yield connection
            connection.commit()
        except Exception:
            connection.rollback()
            raise
        finally:
            connection.close()

    def ensure_session(self, session_id: str, metadata: dict[str, object] | None = None) -> None:
        timestamp = time.time()
        with self._lock, self._connection() as connection:
            connection.execute(
                "INSERT OR IGNORE INTO sessions(id, created_at, updated_at, metadata) VALUES (?, ?, ?, ?)",
                (session_id, timestamp, timestamp, json.dumps(metadata or {}, sort_keys=True)),
            )

    def touch_session(self, session_id: str, status: str = "active") -> None:
        self.ensure_session(session_id)
        with self._lock, self._connection() as connection:
            connection.execute("UPDATE sessions SET updated_at=?, status=? WHERE id=?", (time.time(), status, session_id))

    def append_message(self, session_id: str, role: str, content: str) -> int:
        if role not in {"system", "user", "assistant", "tool"}:
            raise ValueError(f"invalid message role: {role}")
        self.ensure_session(session_id)
        with self._lock, self._connection() as connection:
            cursor = connection.execute(
                "INSERT INTO messages(session_id, role, content, created_at) VALUES (?, ?, ?, ?)",
                (session_id, role, content, time.time()),
            )
            connection.execute("UPDATE sessions SET updated_at=? WHERE id=?", (time.time(), session_id))
            return int(cursor.lastrowid)

    def messages(self, session_id: str, limit: int = 40) -> list[dict[str, str]]:
        self.ensure_session(session_id)
        bounded = max(1, min(int(limit), 200))
        with self._lock, self._connection() as connection:
            rows = connection.execute(
                "SELECT role, content FROM messages WHERE session_id=? ORDER BY id DESC LIMIT ?",
                (session_id, bounded),
            ).fetchall()
        return [{"role": str(row["role"]), "content": str(row["content"])} for row in reversed(rows)]

    def emit(self, aggregate_id: str, kind: str, detail: str, payload: dict[str, object] | None = None) -> dict[str, object]:
        safe_detail = redact_text(detail)
        safe_payload = redact_value(payload or {})
        if not isinstance(safe_payload, dict):
            safe_payload = {}
        with self._lock, self._connection() as connection:
            row = connection.execute("SELECT COALESCE(MAX(sequence), 0) + 1 AS next_seq FROM events WHERE aggregate_id=?", (aggregate_id,)).fetchone()
            sequence = int(row["next_seq"])
            timestamp = time.time()
            connection.execute(
                "INSERT INTO events(aggregate_id, sequence, kind, detail, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)",
                (aggregate_id, sequence, kind, safe_detail, json.dumps(safe_payload, sort_keys=True), timestamp),
            )
        return {"aggregate_id": aggregate_id, "sequence": sequence, "kind": kind, "detail": safe_detail, "payload": safe_payload, "at": timestamp}

    def events(self, aggregate_id: str, after: int = 0, limit: int = 100) -> list[dict[str, object]]:
        bounded = max(1, min(int(limit), 500))
        with self._lock, self._connection() as connection:
            rows = connection.execute(
                "SELECT aggregate_id, sequence, kind, detail, payload, created_at FROM events WHERE aggregate_id=? AND sequence>? ORDER BY sequence LIMIT ?",
                (aggregate_id, int(after), bounded),
            ).fetchall()
        return [
            {"aggregate_id": row["aggregate_id"], "sequence": row["sequence"], "kind": row["kind"], "detail": row["detail"], "payload": json.loads(row["payload"]), "at": row["created_at"]}
            for row in rows
        ]

    def counts(self, session_id: str) -> dict[str, int]:
        with self._lock, self._connection() as connection:
            messages = connection.execute("SELECT COUNT(*) AS count FROM messages WHERE session_id=?", (session_id,)).fetchone()["count"]
            events = connection.execute("SELECT COUNT(*) AS count FROM events WHERE aggregate_id=?", (session_id,)).fetchone()["count"]
        return {"messages": int(messages), "events": int(events)}
