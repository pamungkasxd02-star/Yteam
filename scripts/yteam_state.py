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
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    target TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    phase TEXT NOT NULL DEFAULT 'admitted',
    params TEXT NOT NULL DEFAULT '{}',
    result TEXT NOT NULL DEFAULT '{}',
    error TEXT NOT NULL DEFAULT '',
    worker_id TEXT NOT NULL DEFAULT '',
    attempt INTEGER NOT NULL DEFAULT 0,
    available_at REAL NOT NULL,
    heartbeat_at REAL NOT NULL,
    created_at REAL NOT NULL,
    updated_at REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_queue ON jobs(status, available_at, updated_at);
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

    def latest_session_id(self) -> str | None:
        with self._lock, self._connection() as connection:
            row = connection.execute("SELECT id FROM sessions ORDER BY updated_at DESC, created_at DESC LIMIT 1").fetchone()
        return str(row["id"]) if row else None

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
            connection.execute("BEGIN IMMEDIATE")
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

    def create_job(self, target: str, params: dict[str, object] | None = None, kind: str = "authorized_assessment") -> dict[str, object]:
        import secrets

        job_id = f"job_{int(time.time())}_{secrets.token_hex(5)}"
        timestamp = time.time()
        safe_params = redact_value(params or {})
        if not isinstance(safe_params, dict):
            safe_params = {}
        with self._lock, self._connection() as connection:
            connection.execute(
                "INSERT INTO jobs(id, kind, target, params, available_at, heartbeat_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                (job_id, kind, redact_text(target), json.dumps(safe_params, sort_keys=True), timestamp, timestamp, timestamp, timestamp),
            )
        return self.job(job_id) or {"id": job_id, "target": target, "status": "queued"}

    def job(self, job_id: str) -> dict[str, object] | None:
        with self._lock, self._connection() as connection:
            row = connection.execute("SELECT * FROM jobs WHERE id=?", (job_id,)).fetchone()
        if row is None:
            return None
        return self._job_row(row)

    @staticmethod
    def _job_row(row: sqlite3.Row) -> dict[str, object]:
        return {
            "id": row["id"], "kind": row["kind"], "target": row["target"], "status": row["status"],
            "phase": row["phase"], "params": json.loads(row["params"]), "result": json.loads(row["result"]),
            "error": row["error"], "worker_id": row["worker_id"], "attempt": row["attempt"],
            "available_at": row["available_at"], "heartbeat_at": row["heartbeat_at"],
            "created_at": row["created_at"], "updated_at": row["updated_at"],
        }

    def list_jobs(self, statuses: Iterable[str] | None = None, limit: int = 20) -> list[dict[str, object]]:
        bounded = max(1, min(int(limit), 100))
        values = list(statuses or [])
        with self._lock, self._connection() as connection:
            if values:
                marks = ",".join("?" for _ in values)
                rows = connection.execute(f"SELECT * FROM jobs WHERE status IN ({marks}) ORDER BY updated_at DESC LIMIT ?", (*values, bounded)).fetchall()
            else:
                rows = connection.execute("SELECT * FROM jobs ORDER BY updated_at DESC LIMIT ?", (bounded,)).fetchall()
        return [self._job_row(row) for row in rows]

    def recover_stale_jobs(self, lease_seconds: float = 45.0) -> int:
        cutoff = time.time() - max(5.0, lease_seconds)
        with self._lock, self._connection() as connection:
            cursor = connection.execute(
                "UPDATE jobs SET status='queued', worker_id='', phase='recovered', available_at=?, updated_at=? WHERE status='running' AND heartbeat_at<?",
                (time.time(), time.time(), cutoff),
            )
            return int(cursor.rowcount)

    def claim_job(self, worker_id: str, lease_seconds: float = 45.0) -> dict[str, object] | None:
        now = time.time()
        with self._lock, self._connection() as connection:
            connection.execute("BEGIN IMMEDIATE")
            row = connection.execute(
                "SELECT * FROM jobs WHERE status='queued' AND available_at<=? ORDER BY updated_at, created_at LIMIT 1",
                (now,),
            ).fetchone()
            if row is None:
                return None
            connection.execute(
                "UPDATE jobs SET status='running', worker_id=?, attempt=attempt+1, heartbeat_at=?, updated_at=? WHERE id=? AND status='queued'",
                (worker_id, now, now, row["id"]),
            )
            updated = connection.execute("SELECT * FROM jobs WHERE id=?", (row["id"],)).fetchone()
        return self._job_row(updated) if updated is not None else None

    def heartbeat_job(self, job_id: str, worker_id: str, phase: str | None = None) -> bool:
        now = time.time()
        with self._lock, self._connection() as connection:
            if phase:
                cursor = connection.execute("UPDATE jobs SET heartbeat_at=?, updated_at=?, phase=? WHERE id=? AND status='running' AND worker_id=?", (now, now, phase, job_id, worker_id))
            else:
                cursor = connection.execute("UPDATE jobs SET heartbeat_at=?, updated_at=? WHERE id=? AND status='running' AND worker_id=?", (now, now, job_id, worker_id))
            return cursor.rowcount == 1

    def update_job(self, job_id: str, *, status: str | None = None, phase: str | None = None, result: dict[str, object] | None = None, error: str | None = None, available_at: float | None = None, worker_id: str | None = None) -> dict[str, object] | None:
        allowed = {"queued", "running", "completed", "failed", "cancelled"}
        if status is not None and status not in allowed:
            raise ValueError(f"invalid job status: {status}")
        fields: list[str] = []
        values: list[object] = []
        for name, value in (("status", status), ("phase", phase), ("error", redact_text(error) if error is not None else None), ("worker_id", worker_id), ("available_at", available_at)):
            if value is not None:
                fields.append(f"{name}=?")
                values.append(value)
        if result is not None:
            safe = redact_value(result)
            fields.append("result=?")
            values.append(json.dumps(safe if isinstance(safe, dict) else {}, sort_keys=True))
        if not fields:
            return self.job(job_id)
        fields.append("updated_at=?")
        values.extend([time.time(), job_id])
        with self._lock, self._connection() as connection:
            connection.execute(f"UPDATE jobs SET {', '.join(fields)} WHERE id=?", values)
        return self.job(job_id)
