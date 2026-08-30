"""Persistent LocalSolver task queue.

Tasks are intentionally limited to same-origin browser observation. The queue
survives a service restart, but sensitive browser state is never persisted.
"""

from __future__ import annotations

import json
import secrets
import sqlite3
import threading
import time
from contextlib import contextmanager
from pathlib import Path

from yteam_safety import redact_text


class TaskQueue:
    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.lock = threading.RLock()
        with self.connection() as db:
            db.executescript("""
            PRAGMA journal_mode=WAL;
            CREATE TABLE IF NOT EXISTS tasks (
                id TEXT PRIMARY KEY,
                kind TEXT NOT NULL,
                target TEXT NOT NULL,
                options TEXT NOT NULL DEFAULT '{}',
                status TEXT NOT NULL DEFAULT 'queued',
                result TEXT NOT NULL DEFAULT '{}',
                error TEXT NOT NULL DEFAULT '',
                attempts INTEGER NOT NULL DEFAULT 0,
                created_at REAL NOT NULL,
                updated_at REAL NOT NULL
            );
            CREATE INDEX IF NOT EXISTS idx_localsolver_tasks ON tasks(status, created_at);
            """)

    def connect(self) -> sqlite3.Connection:
        db = sqlite3.connect(self.path, timeout=15, check_same_thread=False)
        db.row_factory = sqlite3.Row
        db.execute("PRAGMA busy_timeout=15000")
        return db

    @contextmanager
    def connection(self):
        db = self.connect()
        try:
            yield db
            db.commit()
        except Exception:
            db.rollback()
            raise
        finally:
            db.close()

    def create(self, target: str, options: dict[str, object] | None = None, kind: str = "browser_observation") -> dict[str, object]:
        task_id = f"ls_{int(time.time())}_{secrets.token_hex(6)}"
        timestamp = time.time()
        with self.lock, self.connection() as db:
            db.execute("INSERT INTO tasks(id, kind, target, options, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", (task_id, kind, redact_text(target), json.dumps(options or {}, sort_keys=True), timestamp, timestamp))
        return self.get(task_id) or {"id": task_id, "status": "queued"}

    def get(self, task_id: str) -> dict[str, object] | None:
        with self.lock, self.connection() as db:
            row = db.execute("SELECT * FROM tasks WHERE id=?", (task_id,)).fetchone()
        if not row:
            return None
        return {"id": row["id"], "kind": row["kind"], "target": row["target"], "options": json.loads(row["options"]), "status": row["status"], "result": json.loads(row["result"]), "error": row["error"], "attempts": row["attempts"], "created_at": row["created_at"], "updated_at": row["updated_at"]}

    def claim(self) -> dict[str, object] | None:
        with self.lock, self.connection() as db:
            db.execute("BEGIN IMMEDIATE")
            row = db.execute("SELECT * FROM tasks WHERE status='queued' ORDER BY created_at LIMIT 1").fetchone()
            if not row:
                return None
            db.execute("UPDATE tasks SET status='running', attempts=attempts+1, updated_at=? WHERE id=? AND status='queued'", (time.time(), row["id"]))
        return self.get(str(row["id"]))

    def finish(self, task_id: str, result: dict[str, object] | None = None, error: str = "") -> dict[str, object] | None:
        status = "failed" if error else "completed"
        with self.lock, self.connection() as db:
            db.execute("UPDATE tasks SET status=?, result=?, error=?, updated_at=? WHERE id=?", (status, json.dumps(result or {}, sort_keys=True), redact_text(error), time.time(), task_id))
        return self.get(task_id)

    def recover_running(self) -> int:
        with self.lock, self.connection() as db:
            cursor = db.execute("UPDATE tasks SET status='queued', updated_at=? WHERE status='running'", (time.time(),))
            return int(cursor.rowcount)

    def list(self, limit: int = 50) -> list[dict[str, object]]:
        bounded = max(1, min(int(limit), 100))
        with self.lock, self.connection() as db:
            rows = db.execute("SELECT id FROM tasks ORDER BY updated_at DESC LIMIT ?", (bounded,)).fetchall()
        return [self.get(str(row["id"])) for row in rows if self.get(str(row["id"])) is not None]
