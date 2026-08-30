#!/usr/bin/env python3
"""Standalone YTEAM runtime services used by the native terminal UI.

The runtime deliberately owns policy, command routing, model selection, session
state, and an append-only event ledger.  It does not import or shell out to an
upstream agent runtime.
"""

from __future__ import annotations

import json
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable

from yteam_ai import stream_chat_events
from yteam_memory import LearningMemory
from yteam_models import discover_free_models, load_model_config
from yteam_session import Session
from yteam_state import StateStore


@dataclass
class RuntimePolicy:
    authorized_only: bool = True
    read_only: bool = True
    max_requests_per_second: float = 1.0
    no_customer_objects: bool = True
    no_destructive_actions: bool = True
    no_auto_submit: bool = True


@dataclass
class RuntimeEvent:
    kind: str
    detail: str
    at: float = field(default_factory=time.time)


class EventLedger:
    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.store = StateStore(path.parent / "state.db")
        self.aggregate_id = "runtime"
        self._lock = threading.RLock()
        self._listeners: list[Callable[[dict[str, object]], None]] = []

    def subscribe(self, listener: Callable[[dict[str, object]], None]) -> None:
        with self._lock:
            self._listeners.append(listener)

    def emit(self, kind: str, detail: str, payload: dict[str, object] | None = None) -> None:
        record = self.store.emit(self.aggregate_id, kind, detail, payload)
        with self._lock, self.path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(record, ensure_ascii=False) + "\n")
            listeners = list(self._listeners)
        for listener in listeners:
            try:
                listener(record)
            except Exception:  # noqa: BLE001
                continue


class YteamRuntime:
    """Own model selection, conversation state, commands, and policy display."""

    def __init__(self, root: Path) -> None:
        self.root = root
        self.runtime_dir = root / "runtime"
        self.config = load_model_config(root)
        self.models = discover_free_models()
        self.selected_model = self.config["model"] if self.config["model"] in self.models else self.models[0]
        self.session = Session.resume_or_new(self.runtime_dir / "sessions")
        self.memory = LearningMemory(self.runtime_dir / "memory" / "learning.jsonl")
        self.events = EventLedger(self.runtime_dir / "events.jsonl")
        self.state = StateStore(self.runtime_dir / "state.db")
        self.policy = RuntimePolicy()
        self.pending_bb_target: str | None = None
        self.quit_requested = False
        self.commands: dict[str, Callable[[str], str]] = {
            "/help": lambda _: self.help_text(),
            "/models": lambda _: self.models_text(),
            "/status": lambda _: self.status_text(),
            "/history": lambda _: self.history_text(),
            "/clear": lambda _: self.clear_text(),
            "/memory": lambda _: self.memory_text(),
            "/events": lambda _: self.events_text(),
            "/jobs": lambda _: self.jobs_text(),
        }

    def help_text(self) -> str:
        return "\n".join([
            "/models                  list the live OpenCode Zen Free catalog",
            "/model <model-id>        select a free model for the next turn",
            "/status                  show runtime, policy, and session state",
            "/history                 show the local conversation summary",
            "/clear                   start a fresh local conversation",
            "/memory                 show verified lessons and pending proposals",
            "/events                  show the latest replayable runtime events",
            "/jobs                    show durable assessment jobs and checkpoints",
            "/learn <lesson>          propose a redacted lesson for verification",
            "/verify <proposal-id>    promote one proposal into verified context",
            "/bb <authorized-target> run the scoped read-only security pipeline",
            "/doctor                  run the local dependency diagnostic",
            "/quit                    exit YTEAM",
        ])

    def models_text(self) -> str:
        lines = ["YTEAM Zen Free models (direct OpenAI-compatible API):"]
        lines.extend(f"{' *' if model == self.selected_model else '  '} {model}" for model in self.models)
        lines.append("Use /model <model-id> to switch.")
        return "\n".join(lines)

    def status_text(self) -> str:
        counts = self.events.store.counts(self.events.aggregate_id)
        jobs = self.state.list_jobs(limit=8)
        return json.dumps({
            "product": "YTEAM",
            "runtime": "standalone",
            "provider": self.config["provider"],
            "model": self.selected_model,
            "session_id": self.session.session_id,
            "message_count": len(self.session.messages),
            "event_count": counts["events"],
            "memory": self.memory.summary(),
            "policy": self.policy.__dict__,
            "jobs": [{"id": job["id"], "status": job["status"], "phase": job["phase"], "target": job["target"]} for job in jobs],
        }, indent=2)

    def history_text(self) -> str:
        if not self.session.messages:
            return "No messages in the current session."
        return "\n".join(f"{item['role']}: {item['content'][:160]}" for item in self.session.messages[-12:])

    def clear_text(self) -> str:
        self.session = Session(self.runtime_dir / "sessions", state_path=self.runtime_dir / "state.db")
        self.events.emit("session.cleared", self.session.session_id)
        return f"Started fresh session: {self.session.session_id}"

    def memory_text(self) -> str:
        summary = self.memory.summary()
        lessons = self.memory.verified(limit=6)
        proposals = self.memory.proposals(limit=4)
        lines = [json.dumps(summary), "Verified lessons:"]
        lines.extend(f"- {item.get('id')}: {item.get('text')}" for item in lessons)
        lines.append("Pending proposals:")
        lines.extend(f"- {item.get('id')}: {item.get('text')}" for item in proposals)
        return "\n".join(lines)

    def events_text(self) -> str:
        events = self.events.store.events(self.events.aggregate_id, after=0, limit=12)
        if not events:
            return "No runtime events recorded."
        return "\n".join(f"#{item['sequence']} {item['kind']}: {item['detail']}" for item in events)

    def jobs_text(self) -> str:
        jobs = self.state.list_jobs(limit=12)
        if not jobs:
            return "No durable assessment jobs."
        return "\n".join(f"{item['id']} [{item['status']}/{item['phase']}] {item['target']} attempt={item['attempt']}" for item in jobs)

    def learn(self, value: str) -> str:
        try:
            item = self.memory.propose(value, source="runtime-command")
        except ValueError as error:
            return f"Learning proposal rejected: {error}"
        self.events.emit("memory.proposed", str(item.get("id")))
        return f"Stored proposal {item.get('id')}. Verify it with /verify {item.get('id')} after evidence review."

    def verify_learning(self, value: str) -> str:
        try:
            item = self.memory.verify(value.strip())
        except ValueError as error:
            return f"Memory verification failed: {error}"
        self.events.emit("memory.verified", str(item.get("id")))
        return f"Verified lesson {item.get('id')} is now available to future model turns."

    def select_model(self, value: str) -> str:
        requested = value.strip()
        if requested not in self.models:
            return f"Unknown free model: {requested}. Use /models."
        self.selected_model = requested
        self.events.emit("model.selected", requested)
        return f"Active model: {requested} (zen-free)"

    def request_bb(self, value: str) -> str:
        target = value.strip()
        if not target or target.startswith("-") or any(char in target for char in "\r\n"):
            return "Usage: /bb <authorized-http(s)-target>"
        job = self.state.create_job(target, {"depth": 2, "rate": self.policy.max_requests_per_second, "use_external": False, "scan": False})
        self.events.emit("bb.admitted", str(job["id"]), {"target": target, "job_id": job["id"]})
        return f"Queued durable read-only assessment {job['id']}: {target}\nThe worker will resume it after a terminal close."

    def command(self, message: str) -> str | None:
        message = message.strip()
        if message in self.commands:
            return self.commands[message](message)
        if message.startswith("/model "):
            return self.select_model(message[7:])
        if message.startswith("/learn "):
            return self.learn(message[7:])
        if message.startswith("/verify "):
            return self.verify_learning(message[8:])
        if message.startswith("/doctor"):
            from yteam_doctor import run
            return json.dumps(run(), indent=2)
        if message == "/quit":
            self.quit_requested = True
            self.events.emit("runtime.quit", self.session.session_id)
            return "Goodbye."
        if message.startswith("/bb") and (message == "/bb" or message.startswith("/bb ")):
            return self.request_bb(message[3:])
        return None

    def answer(self, message: str) -> str:
        return "".join(self.answer_stream(message))

    def answer_stream(self, message: str):
        self.session.append("user", message)
        self.events.emit("chat.requested", self.selected_model)
        prompt = [{"role": "system", "content": "You are YTEAM. Use only verified lessons as prior knowledge; treat proposals and hypotheses as unverified. Follow the active safety policy and never invent evidence.\n\nVerified YTEAM lessons:\n" + self.memory.context(message)}]
        prompt.extend(self.session.conversation())
        chunks: list[str] = []
        try:
            for event in stream_chat_events({**self.config, "model": self.selected_model}, prompt):
                kind = str(event.get("type", ""))
                self.events.emit(kind, self.selected_model, {key: value for key, value in event.items() if key != "type"})
                if kind == "message.delta":
                    chunk = str(event.get("text", ""))
                    chunks.append(chunk)
                    yield chunk
        except RuntimeError as error:
            self.events.emit("provider.error", str(error))
            raise
        response = "".join(chunks)
        self.session.append("assistant", response)
        self.events.emit("chat.completed", self.selected_model)

    def snapshot(self) -> dict[str, object]:
        return {"model": self.selected_model, "provider": self.config["provider"], "session_id": self.session.session_id, "message_count": len(self.session.messages), "event_count": self.events.store.counts(self.events.aggregate_id)["events"], "memory": self.memory.summary(), "policy": self.policy.__dict__}
