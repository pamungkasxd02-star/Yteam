"""LocalSolver asynchronous observation service built around Camoufox."""

from __future__ import annotations

import os
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from urllib.parse import urlsplit

from .camoufox_adapter import CamoufoxConfig, run_camoufox
from .task_queue import TaskQueue


def allowed_target(target: str, allowlist: set[str]) -> bool:
    parsed = urlsplit(target)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        return False
    # The service is local, but a remote caller still needs an explicit exact
    # target allowlist. This prevents the service becoming an SSRF proxy.
    return target.rstrip("/") in {item.rstrip("/") for item in allowlist}


class LocalSolverService:
    def __init__(self, root: Path, workers: int = 2) -> None:
        self.root = root
        self.output = root / "runtime" / "localsolver"
        self.queue = TaskQueue(root / "runtime" / "localsolver" / "tasks.db")
        self.allowlist = {item.strip() for item in os.environ.get("LOCALSOLVER_TARGET_ALLOWLIST", "").split(",") if item.strip()}
        self.workers = max(1, min(int(workers), 4))
        self.pool = ThreadPoolExecutor(max_workers=self.workers, thread_name_prefix="localsolver")
        self.stop = threading.Event()
        self.queue.recover_running()
        self.thread = threading.Thread(target=self._dispatch, name="localsolver-dispatch", daemon=True)
        self.thread.start()

    def submit(self, target: str, options: dict[str, object] | None = None) -> dict[str, object]:
        if not self.allowlist:
            raise PermissionError("LOCALSOLVER_TARGET_ALLOWLIST is not configured")
        if not allowed_target(target, self.allowlist):
            raise PermissionError("target is not in LOCALSOLVER_TARGET_ALLOWLIST")
        options = options or {}
        safe_options = {"headless": bool(options.get("headless", True)), "timeout_ms": max(3_000, min(int(options.get("timeout_ms", 12_000)), 30_000)), "rate": max(0.1, min(float(options.get("rate", 1.0)), 5.0))}
        task = self.queue.create(target.rstrip("/"), safe_options)
        return {"task_id": task["id"], "status": task["status"], "policy": "allowlisted browser observation; no token/cookie export, CAPTCHA solving, proxy rotation, or WAF evasion"}

    def result(self, task_id: str) -> dict[str, object] | None:
        task = self.queue.get(task_id)
        if task is None:
            return None
        return {"task_id": task["id"], "status": task["status"], "elapsed_time": max(0.0, time.time() - float(task["created_at"])), "value": task["result"] if task["status"] == "completed" else None, "error": task["error"] or None, "attempts": task["attempts"]}

    def close(self) -> None:
        self.stop.set()
        self.thread.join(timeout=2)
        self.pool.shutdown(wait=False, cancel_futures=True)

    def _dispatch(self) -> None:
        while not self.stop.is_set():
            task = self.queue.claim()
            if task is None:
                self.stop.wait(0.5)
                continue
            self.pool.submit(self._execute, task)

    def _execute(self, task: dict[str, object]) -> None:
        task_id = str(task["id"])
        options = dict(task.get("options") or {})
        output = self.output / task_id
        try:
            result = run_camoufox(CamoufoxConfig(str(task["target"]), output, headless=bool(options.get("headless", True)), timeout_ms=int(options.get("timeout_ms", 12_000)), rate=float(options.get("rate", 1.0))))
            self.queue.finish(task_id, {"engine": "localsolver-camoufox", "output": str(output), "observation": result})
        except Exception as error:  # noqa: BLE001
            self.queue.finish(task_id, error=f"{type(error).__name__}: {error}")
