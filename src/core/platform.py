"""Unified Yteam Platform control plane.

All pillars use these contracts. Engines return structured, evidence-linked
results; the control plane owns policy, events, artifacts, and lifecycle.
No engine is allowed to turn a signal into a finding by itself.
"""

from __future__ import annotations

import threading
import uuid
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Iterable

from . import now_iso, redact, write_json


@dataclass(frozen=True)
class Policy:
    authorized: bool = True
    read_only: bool = True
    max_requests_per_second: float = 5.0
    allow_external_tools: bool = True
    allow_customer_objects: bool = False
    allow_destructive_actions: bool = False
    allow_denial_of_service: bool = False
    allow_credential_stuffing: bool = False

    def permits(self, action: str) -> bool:
        if not self.authorized:
            return False
        if action in {"read", "recon", "analyze", "evidence"}:
            return True
        if action == "external_tool":
            return self.allow_external_tools
        if action == "destructive":
            return self.allow_destructive_actions and not self.read_only
        if action == "dos":
            return self.allow_denial_of_service and not self.read_only
        if action == "credential_stuffing":
            return self.allow_credential_stuffing and not self.read_only
        if action == "customer_object":
            return self.allow_customer_objects
        return False


@dataclass
class Event:
    name: str
    phase: str
    detail: dict[str, Any] = field(default_factory=dict)
    at: str = field(default_factory=now_iso)
    event_id: str = field(default_factory=lambda: uuid.uuid4().hex[:16])


class EventBus:
    """Thread-safe in-process event bus for engine coordination."""

    def __init__(self) -> None:
        self.events: list[Event] = []
        self._subscribers: dict[str, list[Callable[[Event], None]]] = {}
        self._lock = threading.Lock()

    def subscribe(self, name: str, callback: Callable[[Event], None]) -> None:
        with self._lock:
            self._subscribers.setdefault(name, []).append(callback)

    def emit(self, name: str, phase: str, detail: dict[str, Any] | None = None) -> Event:
        event = Event(name=name, phase=phase, detail=redact(detail or {}))
        with self._lock:
            self.events.append(event)
            callbacks = list(self._subscribers.get(name, [])) + list(self._subscribers.get("*", []))
        for callback in callbacks:
            callback(event)
        return event


class ArtifactStore:
    """Target/run-scoped artifact writer with safe JSON serialization."""

    def __init__(self, root: Path) -> None:
        self.root = root.resolve()
        self.root.mkdir(parents=True, exist_ok=True)

    def path(self, relative: str) -> Path:
        candidate = (self.root / relative).resolve()
        if self.root != candidate and self.root not in candidate.parents:
            raise ValueError("artifact path escapes run directory")
        return candidate

    def json(self, relative: str, value: object) -> Path:
        path = self.path(relative)
        write_json(path, value)
        return path

    def text(self, relative: str, value: str) -> Path:
        path = self.path(relative)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(str(redact(value)), encoding="utf-8")
        return path


@dataclass
class EngineResult:
    engine: str
    status: str
    summary: str
    artifacts: list[str] = field(default_factory=list)
    signals: list[str] = field(default_factory=list)
    next_actions: list[str] = field(default_factory=list)
    error: str | None = None


@dataclass
class AssessmentContext:
    run_id: str
    target: str
    target_slug: str
    artifacts: ArtifactStore
    policy: Policy = field(default_factory=Policy)
    events: EventBus = field(default_factory=EventBus)
    state: dict[str, Any] = field(default_factory=dict)
    env: dict[str, str] = field(default_factory=lambda: dict(os.environ))

    def emit(self, name: str, phase: str, **detail: Any) -> Event:
        return self.events.emit(name, phase, detail)

    def require(self, action: str) -> None:
        if not self.policy.permits(action):
            raise PermissionError(f"Yteam policy denied action: {action}")


Engine = Callable[[AssessmentContext], EngineResult]


class EngineRegistry:
    """Registry for pluggable Yteam engines."""

    def __init__(self) -> None:
        self._engines: dict[str, Engine] = {}

    def register(self, name: str, engine: Engine) -> None:
        if not name or name in self._engines:
            raise ValueError(f"invalid or duplicate engine: {name}")
        self._engines[name] = engine

    def get(self, name: str) -> Engine:
        return self._engines[name]

    def names(self) -> tuple[str, ...]:
        return tuple(sorted(self._engines))


def serialize_events(events: Iterable[Event]) -> list[dict[str, Any]]:
    return [redact({"event_id": item.event_id, "name": item.name, "phase": item.phase, "detail": item.detail, "at": item.at}) for item in events]
