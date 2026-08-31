#!/usr/bin/env python3
"""Durable background worker for YTEAM assessment jobs.

The worker is intentionally boring: claim one SQLite job, checkpoint the
pipeline run ID, heartbeat while the native hunt executes, and leave enough
state for the next worker to recover after a crash or terminal close.
"""

from __future__ import annotations

import argparse
import json
import os
import secrets
import subprocess
import sys
import threading
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))


def _store():
    from yteam_state import StateStore

    return StateStore(ROOT / "runtime" / "state.db")


def ensure_worker(root: Path = ROOT) -> bool:
    """Start one detached worker if no live worker lock is present."""
    worker_script = root / "scripts" / "yteam_worker.py"
    if not worker_script.exists():
        return False
    runtime = root / "runtime"
    lock = runtime / "worker.lock"
    if lock.exists():
        live_pid = 0
        try:
            first_line = lock.read_text(encoding="utf-8", errors="replace").splitlines()[0]
            if first_line.startswith("pid="):
                live_pid = int(first_line[4:])
        except (OSError, ValueError, IndexError):
            live_pid = 0
        if live_pid and _process_alive(live_pid):
            return False
        try:
            if lock.stat().st_mtime > time.time() - 60:
                return False
        except OSError:
            return False
        try:
            lock.unlink()
        except OSError:
            return False
    runtime.mkdir(parents=True, exist_ok=True)
    log = runtime / "worker.log"
    output = log.open("a", encoding="utf-8")
    kwargs: dict[str, object] = {"cwd": str(root), "stdin": subprocess.DEVNULL, "stdout": output, "stderr": subprocess.STDOUT, "close_fds": True}
    if os.name == "nt":
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP | subprocess.DETACHED_PROCESS
    else:
        kwargs["start_new_session"] = True
    try:
        subprocess.Popen([sys.executable, str(worker_script)], **kwargs)
    except OSError:
        output.close()
        return False
    output.close()
    return True


def _process_alive(pid: int) -> bool:
    """Portable liveness probe used only for the local worker lock."""
    if pid <= 0:
        return False
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    except OSError:
        return False
    return True


def _prepare_or_resume(store, job: dict[str, object]) -> tuple[str, Path]:
    import bb_pipeline

    bb_pipeline.RUNS = ROOT / "runtime" / "bb-runs"
    params = dict(job.get("params") or {})
    saved_result = dict(job.get("result") or {})
    pipeline_id = str(params.get("pipeline_run_id") or saved_result.get("pipeline_run_id") or "")
    if pipeline_id:
        try:
            ledger = bb_pipeline.read(pipeline_id)
        except FileNotFoundError:
            ledger = bb_pipeline.prepare(str(job["target"]), pipeline_id)
    else:
        pipeline_id = "bb_job_" + str(job["id"])[4:]
        ledger = bb_pipeline.prepare(str(job["target"]), pipeline_id)
        params["pipeline_run_id"] = ledger["run_id"]
        store.update_job(str(job["id"]), phase="scope", result={"pipeline_run_id": ledger["run_id"], "target_slug": ledger.get("target_slug", "")})
        job["params"] = params
    relative = Path(str(ledger["paths"]["recon"]))
    output = relative if relative.is_absolute() else ROOT / relative
    output.mkdir(parents=True, exist_ok=True)
    return str(ledger["run_id"]), output


def execute_job(store, job: dict[str, object], worker_id: str) -> dict[str, object]:
    job_id = str(job["id"])
    target = str(job["target"])
    # Policy gate: refuse to run a target that exceeds the active engine policy.
    try:
        from yteam_engine import Engine

        engine = Engine(ROOT)
        engine.policy.assert_effect("read", target)
    except Exception as error:  # noqa: BLE001
        store.update_job(job_id, status="failed", phase="policy_blocked", error=f"policy: {error}", worker_id="")
        return {"pipeline_run_id": "", "output": "", "hunt_status": "policy_blocked", "stages": 0, "error": str(error)}
    pipeline_id, output = _prepare_or_resume(store, job)
    stop = threading.Event()

    def heartbeat() -> None:
        while not stop.wait(5):
            store.heartbeat_job(job_id, worker_id, "recon")
            try:
                (ROOT / "runtime" / "worker.lock").touch()
            except OSError:
                pass

    thread = threading.Thread(target=heartbeat, name=f"heartbeat-{job_id}", daemon=True)
    thread.start()
    store.update_job(job_id, phase="recon")
    try:
        params = dict(job.get("params") or {})
        if str(job.get("kind")) == "autonomous_assessment":
            from yteam_autonomy import run as run_autonomy

            store.update_job(job_id, phase="autonomy")
            result = run_autonomy(target, output, pipeline_id, params, store, job_id)
            successful = result.get("status") == "completed"
            summary = {
                "pipeline_run_id": pipeline_id,
                "output": str(output),
                "autonomy_status": result.get("status"),
                "rounds": result.get("rounds", 0),
                "actions": len(result.get("results", [])),
                "stop_reason": result.get("stop_reason", ""),
            }
            store.update_job(
                job_id,
                status="completed" if successful else "failed",
                phase="triage" if successful else "blocked",
                result=summary,
                error="" if successful else str(result.get("stop_reason", "autonomous assessment blocked")),
            )
        else:
            from yteam_hunt import run as run_hunt

            result = run_hunt(
                target, output, pipeline_id,
                int(params.get("depth", 2)), float(params.get("rate", 1.0)),
                bool(params.get("use_external", False)), bool(params.get("scan", False)),
                Path(str(params["scope_file"])) if params.get("scope_file") else None,
            )
            successful = result.get("status") != "blocked"
            summary = {"pipeline_run_id": pipeline_id, "output": str(output), "hunt_status": result.get("status"), "stages": len(result.get("stages", []))}
            store.update_job(job_id, status="completed" if successful else "failed", phase="triage", result=summary, error="" if successful else "scope or recon blocked")
        _log_knowledge(target, result)
        return summary
    finally:
        stop.set()
        thread.join(timeout=2)


def _log_knowledge(target: str, result: dict[str, object]) -> None:
    """Best-effort record the run into the durable knowledge graph."""
    try:
        from yteam_engine import Engine

        engine = Engine(ROOT)
        engine.knowledge.add_node("target", target, {"hunt_status": result.get("status"), "stages": len(result.get("stages", []))})
    except Exception:  # noqa: BLE001 - knowledge logging must never break the worker
        pass


def run_once(store, worker_id: str) -> bool:
    store.recover_stale_jobs()
    job = store.claim_job(worker_id)
    if not job:
        return False
    try:
        execute_job(store, job, worker_id)
    except Exception as error:  # noqa: BLE001
        attempts = int(job.get("attempt", 1))
        if attempts < 3:
            store.update_job(str(job["id"]), status="queued", phase="retry_wait", error=f"{type(error).__name__}: {error}", available_at=time.time() + (attempts * 10), worker_id="")
        else:
            store.update_job(str(job["id"]), status="failed", phase="failed", error=f"{type(error).__name__}: {error}", worker_id="")
    return True


def daemon() -> int:
    lock_path = ROOT / "runtime" / "worker.lock"
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    try:
        handle = lock_path.open("x", encoding="utf-8")
    except FileExistsError:
        return 0
    worker_id = f"worker_{os.getpid()}_{secrets.token_hex(3)}"
    handle.write(f"pid={os.getpid()}\nworker={worker_id}")
    handle.close()
    try:
        store = _store()
        while True:
            try:
                lock_path.touch()
            except OSError:
                pass
            if not run_once(store, worker_id):
                time.sleep(3)
    except KeyboardInterrupt:
        return 0
    finally:
        try:
            lock_path.unlink()
        except OSError:
            pass


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--once", action="store_true")
    args = parser.parse_args()
    store = _store()
    worker_id = f"worker_{os.getpid()}_{secrets.token_hex(3)}"
    if args.once:
        return 0 if run_once(store, worker_id) else 0
    return daemon()


if __name__ == "__main__":
    raise SystemExit(main())
