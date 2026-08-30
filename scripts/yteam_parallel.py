#!/usr/bin/env python3
"""Rate-limited parallel recon scheduler for Yteam.

Runs multiple read-only probe tasks concurrently while enforcing a global
requests-per-second budget and a concurrency cap. Never performs destructive
or authenticated writes; used only for authorized, in-scope reconnaissance.
"""

from __future__ import annotations

import argparse
import json
import threading
import time
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


@dataclass
class RateLimiter:
    rate: float = 5.0
    _lock: threading.Lock = field(default_factory=threading.Lock)
    _timestamps: deque = field(default_factory=deque)

    def wait(self) -> None:
        window = max(1.0, 5.0)
        with self._lock:
            now = time.monotonic()
            while self._timestamps and self._timestamps[0] <= now - window:
                self._timestamps.popleft()
            if len(self._timestamps) >= max(1, int(self.rate * window)):
                delay = (self._timestamps[0] + window) - now
                time.sleep(max(0.0, delay))
                now = time.monotonic()
            self._timestamps.append(now)


@dataclass
class Task:
    name: str
    fn: object
    args: tuple = ()
    kwargs: dict = field(default_factory=dict)


class ParallelScheduler:
    def __init__(self, max_workers: int = 4, rate: float = 5.0) -> None:
        self.max_workers = max(1, min(max_workers, 16))
        self.rate = rate
        self.limiter = RateLimiter(rate)
        self.results: dict[str, object] = {}

    def run(self, tasks: list[Task]) -> dict[str, object]:
        def _wrapped(task: Task) -> tuple[str, object]:
            self.limiter.wait()
            try:
                return task.name, task.fn(*task.args, **task.kwargs)
            except Exception as error:  # noqa: BLE001
                return task.name, {"error": str(error)}

        with ThreadPoolExecutor(max_workers=self.max_workers) as executor:
            futures = [executor.submit(_wrapped, task) for task in tasks]
            for future in futures:
                name, result = future.result()
                self.results[name] = result
        return self.results


def _sample_probe(url: str) -> dict:
    # Minimal read-only sample for demonstration/testing; real recon uses
    # yteam_recon/probe engines through Hermes terminal.
    import socket

    host = url.split("://")[-1].split("/")[0].split(":")[0]
    try:
        socket.getaddrinfo(host, None)
        return {"url": url, "status": "resolvable"}
    except Exception as error:  # noqa: BLE001
        return {"url": url, "status": "error", "detail": str(error)}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--targets", nargs="+", required=True)
    parser.add_argument("--workers", type=int, default=4)
    parser.add_argument("--rate", type=float, default=5.0)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    scheduler = ParallelScheduler(max_workers=args.workers, rate=args.rate)
    tasks = [Task(name=f"probe-{index}", fn=_sample_probe, args=(target,)) for index, target in enumerate(args.targets)]
    results = scheduler.run(tasks)
    payload = {"engine": "yteam-parallel", "rate_limit_rps": args.rate, "max_workers": args.workers, "results": results, "non_claim": "Parallel recon is read-only surface mapping, not vulnerability proof."}
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(payload, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
