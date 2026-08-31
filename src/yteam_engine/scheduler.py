"""Durable multi-target scheduler for YTEAM.

The scheduler layers a priority queue and per-target token-bucket rate limiting
on top of the existing SQLite job store. It enforces:
- a global concurrency budget (workers),
- a per-target requests-per-second ceiling (token bucket),
- anti-thrash (a target that recently completed or hit a blocker is cooled off
  so the autonomous loop does not re-queue it endlessly),
- deny-by-default policy before a target is admitted.

The scheduler never submits reports and never runs destructive actions; those
remain gated by the caller.
"""

from __future__ import annotations

import threading
import time
from dataclasses import dataclass, field
from typing import Iterable, Protocol

from .policy import Policy, PolicyViolation


class JobStore(Protocol):
    """Minimal job-store surface the scheduler depends on."""

    def create_job(self, target: str, params: dict | None, kind: str) -> dict[str, object]: ...
    def list_jobs(self, statuses: Iterable[str] | None = None, limit: int = 20) -> list[dict[str, object]]: ...


class TokenBucket:
    """A simple per-target token bucket with burst allowance."""

    def __init__(self, rate_per_second: float, burst: int = 1) -> None:
        self.rate = max(0.0, float(rate_per_second))
        self.burst = max(1, int(burst))
        self._tokens = float(self.burst)
        self._updated = time.monotonic()

    def acquire(self, now: float | None = None) -> bool:
        now = now if now is not None else time.monotonic()
        self._tokens = min(self.burst, self._tokens + (now - self._updated) * self.rate)
        self._updated = now
        if self._tokens >= 1.0:
            self._tokens -= 1.0
            return True
        return False

    def next_available(self, now: float | None = None) -> float:
        now = now if now is not None else time.monotonic()
        shortage = 1.0 - self._tokens
        if shortage <= 0:
            return 0.0
        if self.rate <= 0:
            return float("inf")
        return shortage / self.rate


@dataclass
class TargetEntry:
    """Bookkeeping for one admitted target."""

    target: str
    priority: int = 0  # lower number = higher priority
    admitted_at: float = field(default_factory=time.time)
    last_run_at: float = 0.0
    run_count: int = 0
    cool_down_until: float = 0.0
    rate_per_second: float = 1.0
    blocked: bool = False
    block_reason: str = ""


@dataclass
class Admission:
    admitted: bool
    reason: str
    job: dict[str, object] | None = None


class Scheduler:
    """Priority-aware, rate-limited, anti-thrash target scheduler."""

    def __init__(self, policy: Policy, store: JobStore | None = None, max_workers: int = 2, cool_down_seconds: float = 300.0) -> None:
        self.policy = policy
        self.store = store
        self.max_workers = max(1, int(max_workers))
        self.cool_down_seconds = max(0.0, float(cool_down_seconds))
        self._lock = threading.RLock()
        self._targets: dict[str, TargetEntry] = {}
        self._buckets: dict[str, TokenBucket] = {}

    # -- target registration / bookkeeping --------------------------------
    def register(self, target: str, priority: int = 0, rate_per_second: float | None = None) -> TargetEntry:
        """Register a target for scheduling. Re-registering updates priority."""
        with self._lock:
            existing = self._targets.get(target)
            if existing:
                existing.priority = priority
                if rate_per_second is not None:
                    existing.rate_per_second = rate_per_second
                return existing
            budget = self.policy.budget_for(target)
            entry = TargetEntry(
                target=target,
                priority=priority,
                rate_per_second=rate_per_second if rate_per_second is not None else budget.rate_per_second,
            )
            self._targets[target] = entry
            self._buckets[target] = TokenBucket(entry.rate_per_second)
            return entry

    def admit(self, target: str, priority: int = 0, params: dict | None = None) -> Admission:
        """Policy-check and queue a target. Returns an Admission record."""
        if not target or not target.strip():
            return Admission(False, "target is empty")
        entry = self.register(target, priority)
        now = time.time()
        with self._lock:
            if entry.blocked:
                return Admission(False, f"target blocked: {entry.block_reason}")
            if now < entry.cool_down_until:
                remaining = int(entry.cool_down_until - now)
                return Admission(False, f"target in cool-down for {remaining}s (anti-thrash)")
            # policy gate: deny-by-default before admission
            try:
                self.policy.assert_effect("read", target)
            except PolicyViolation as error:
                entry.blocked = True
                entry.block_reason = str(error)
                return Admission(False, str(error))
        if self.store is None:
            return Admission(True, "admitted (no job store bound)")
        job = self.store.create_job(target, params, "authorized_assessment")
        return Admission(True, "queued", job)

    # -- scheduling / rate limiting ---------------------------------------
    def _running_count(self) -> int:
        if self.store is None:
            return 0
        running = self.store.list_jobs(statuses=["running"], limit=100)
        return len(running)

    def acquire_slot(self, target: str) -> bool:
        """Try to grab a concurrency slot and a rate token for ``target``."""
        with self._lock:
            if self._running_count() >= self.max_workers:
                return False
            entry = self._targets.get(target)
            if entry is None or entry.blocked:
                return False
            bucket = self._buckets.setdefault(target, TokenBucket(entry.rate_per_second))
            if not bucket.acquire():
                return False
            entry.last_run_at = time.time()
            entry.run_count += 1
            return True

    def complete(self, target: str, success: bool) -> None:
        """Record a finished run and apply cool-down to prevent thrash."""
        now = time.time()
        with self._lock:
            entry = self._targets.get(target)
            if entry is None:
                return
            if success:
                entry.cool_down_until = now + self.cool_down_seconds
            else:
                entry.cool_down_until = now + (self.cool_down_seconds * 2)

    def mark_blocked(self, target: str, reason: str) -> None:
        with self._lock:
            entry = self._targets.setdefault(
                target, TargetEntry(target=target, rate_per_second=self.policy.budget_for(target).rate_per_second)
            )
            entry.blocked = True
            entry.block_reason = reason

    def next_target(self) -> str | None:
        """Return the highest-priority, ready, not-cooling target, or None."""
        now = time.time()
        with self._lock:
            ready = [
                entry
                for entry in self._targets.values()
                if not entry.blocked and now >= entry.cool_down_until
            ]
            if not ready:
                return None
            ready.sort(key=lambda entry: (entry.priority, entry.admitted_at))
            return ready[0].target

    def summary(self) -> dict[str, object]:
        with self._lock:
            return {
                "target_count": len(self._targets),
                "max_workers": self.max_workers,
                "cool_down_seconds": self.cool_down_seconds,
                "targets": [
                    {
                        "target": entry.target,
                        "priority": entry.priority,
                        "run_count": entry.run_count,
                        "blocked": entry.blocked,
                        "cooling": time.time() < entry.cool_down_until,
                        "rate_per_second": entry.rate_per_second,
                    }
                    for entry in sorted(self._targets.values(), key=lambda item: (item.priority, item.admitted_at))
                ],
            }
