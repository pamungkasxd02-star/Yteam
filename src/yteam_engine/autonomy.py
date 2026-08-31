"""Policy-bound autonomous action loop for YTEAM.

The loop is deliberately small and auditable.  A planner emits named actions,
the registry resolves those names to reviewed Python callables, and every call
passes through policy, approval, timeout, redaction, and output-size gates.
Model text is never executed directly and cannot invent a new tool at runtime.
"""

from __future__ import annotations

import hashlib
import json
import threading
import time
from concurrent.futures import ThreadPoolExecutor, TimeoutError as FutureTimeout
from dataclasses import asdict, dataclass, field
from typing import Callable, Iterable, Protocol

from .policy import Policy, PolicyViolation, SIDE_EFFECT_RANK


ToolHandler = Callable[[dict[str, object]], dict[str, object]]
EventHandler = Callable[[str, str, dict[str, object]], None]
CheckpointHandler = Callable[[dict[str, object]], None]
CancelHandler = Callable[[], bool]
ReplanHandler = Callable[["AgentRun", list["Action"]], Iterable["Action"]]


class ApprovalStore(Protocol):
    """Narrow durable-store contract used by the engine."""

    def create_approval(
        self,
        tool_name: str,
        target: str,
        reason: str,
        arguments: dict[str, object],
        *,
        job_id: str = "",
        action_id: str = "",
    ) -> dict[str, object]: ...

    def approval(self, approval_id: str) -> dict[str, object] | None: ...


@dataclass(frozen=True)
class ToolSpec:
    name: str
    description: str
    side_effect: str
    handler: ToolHandler = field(repr=False, compare=False)
    requires_approval: bool = False
    timeout_seconds: float = 30.0
    max_output_bytes: int = 64_000

    def __post_init__(self) -> None:
        if not self.name or any(char.isspace() for char in self.name):
            raise ValueError("tool name must be a non-empty token")
        if self.side_effect not in SIDE_EFFECT_RANK:
            raise ValueError(f"unknown tool side effect: {self.side_effect!r}")
        if self.timeout_seconds <= 0 or self.timeout_seconds > 600:
            raise ValueError("tool timeout must be within (0, 600] seconds")
        if self.max_output_bytes < 256 or self.max_output_bytes > 1_000_000:
            raise ValueError("tool output limit must be within [256, 1000000] bytes")


@dataclass(frozen=True)
class Action:
    id: str
    tool: str
    arguments: dict[str, object] = field(default_factory=dict)
    depends_on: tuple[str, ...] = ()
    objective: str = ""

    def as_dict(self) -> dict[str, object]:
        return {
            "id": self.id,
            "tool": self.tool,
            "arguments": dict(self.arguments),
            "depends_on": list(self.depends_on),
            "objective": self.objective,
        }

    @classmethod
    def from_dict(cls, value: dict[str, object]) -> "Action":
        return cls(
            id=str(value.get("id", "")),
            tool=str(value.get("tool", "")),
            arguments=dict(value.get("arguments", {})),
            depends_on=tuple(str(item) for item in value.get("depends_on", [])),
            objective=str(value.get("objective", "")),
        )


@dataclass
class ToolResult:
    action_id: str
    tool: str
    status: str
    observation: dict[str, object] = field(default_factory=dict)
    error: str = ""
    approval_id: str = ""
    duration_seconds: float = 0.0

    def as_dict(self) -> dict[str, object]:
        return asdict(self)


@dataclass
class AgentRun:
    target: str
    status: str = "running"
    stop_reason: str = ""
    rounds: int = 0
    results: list[ToolResult] = field(default_factory=list)
    started_at: float = field(default_factory=time.time)
    finished_at: float = 0.0
    generation: int = 0

    def as_dict(self) -> dict[str, object]:
        return {
            "target": self.target,
            "status": self.status,
            "stop_reason": self.stop_reason,
            "rounds": self.rounds,
            "results": [item.as_dict() for item in self.results],
            "started_at": self.started_at,
            "finished_at": self.finished_at,
            "generation": self.generation,
        }


def _bounded(value: object, limit: int) -> dict[str, object]:
    """Return a JSON-safe, bounded observation without leaking huge outputs."""
    try:
        encoded = json.dumps(value, ensure_ascii=False, default=str, sort_keys=True)
    except (TypeError, ValueError):
        encoded = json.dumps({"value": str(value)}, ensure_ascii=False)
    raw = encoded.encode("utf-8", errors="replace")
    if len(raw) <= limit:
        decoded = json.loads(encoded)
        return decoded if isinstance(decoded, dict) else {"value": decoded}
    digest = hashlib.sha256(raw).hexdigest()
    preview = raw[: max(0, limit - 256)].decode("utf-8", errors="replace")
    return {
        "truncated": True,
        "original_bytes": len(raw),
        "sha256": digest,
        "preview": preview,
    }


class ToolRegistry:
    """Reviewed tool registry with mandatory execution gates."""

    def __init__(self, policy: Policy, approval_store: ApprovalStore | None = None) -> None:
        self.policy = policy
        self.approval_store = approval_store
        self._tools: dict[str, ToolSpec] = {}
        self._lock = threading.RLock()

    def register(self, spec: ToolSpec) -> None:
        with self._lock:
            if spec.name in self._tools:
                raise ValueError(f"tool already registered: {spec.name}")
            self._tools[spec.name] = spec

    def names(self) -> list[str]:
        with self._lock:
            return sorted(self._tools)

    def describe(self) -> list[dict[str, object]]:
        with self._lock:
            return [
                {
                    "name": item.name,
                    "description": item.description,
                    "side_effect": item.side_effect,
                    "requires_approval": item.requires_approval,
                    "timeout_seconds": item.timeout_seconds,
                    "max_output_bytes": item.max_output_bytes,
                }
                for item in sorted(self._tools.values(), key=lambda value: value.name)
            ]

    def execute(self, action: Action, target: str, *, approval_id: str = "", context: dict[str, object] | None = None) -> ToolResult:
        started = time.monotonic()
        with self._lock:
            spec = self._tools.get(action.tool)
        if spec is None:
            return ToolResult(action.id, action.tool, "blocked", error="tool is not registered")
        try:
            self.policy.assert_effect(spec.side_effect, target)
        except PolicyViolation as error:
            return ToolResult(action.id, action.tool, "blocked", error=str(error))

        execution_context = dict(context or {})
        job_id = str(execution_context.get("job_id", ""))
        if spec.requires_approval:
            if self.approval_store and not approval_id and job_id:
                finder = getattr(self.approval_store, "approval_for_action", None)
                existing = finder(job_id, action.id) if callable(finder) else None
                if existing and existing.get("status") == "denied":
                    return ToolResult(action.id, action.tool, "blocked", error="operator denied approval", approval_id=str(existing["id"]))
                if existing and existing.get("status") == "approved" and not existing.get("consumed_at"):
                    approval_id = str(existing["id"])
            approval = self.approval_store.approval(approval_id) if self.approval_store and approval_id else None
            valid = bool(
                approval
                and approval.get("status") == "approved"
                and approval.get("tool_name") == spec.name
                and approval.get("target") == target
            )
            if not valid:
                if self.approval_store is None:
                    return ToolResult(action.id, action.tool, "blocked", error="approval store unavailable")
                request = self.approval_store.create_approval(
                    spec.name,
                    target,
                    action.objective or spec.description,
                    action.arguments,
                    job_id=job_id,
                    action_id=action.id,
                )
                return ToolResult(
                    action.id,
                    action.tool,
                    "approval_required",
                    error="operator approval required",
                    approval_id=str(request["id"]),
                )

        payload = {
            "target": target,
            "arguments": dict(action.arguments),
            "context": execution_context,
            "action_id": action.id,
        }
        executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix=f"yteam-{spec.name}")
        future = executor.submit(spec.handler, payload)
        try:
            output = future.result(timeout=spec.timeout_seconds)
            if not isinstance(output, dict):
                raise TypeError(f"tool returned {type(output).__name__}, expected dict")
            observation = _bounded(output, spec.max_output_bytes)
            if approval_id and self.approval_store:
                consume = getattr(self.approval_store, "consume_approval", None)
                if callable(consume):
                    consume(approval_id)
            return ToolResult(
                action.id,
                action.tool,
                "completed",
                observation=observation,
                duration_seconds=time.monotonic() - started,
            )
        except FutureTimeout:
            future.cancel()
            executor.shutdown(wait=False, cancel_futures=True)
            return ToolResult(
                action.id,
                action.tool,
                "failed",
                error=f"tool timed out after {spec.timeout_seconds:g}s",
                duration_seconds=time.monotonic() - started,
            )
        except Exception as error:  # noqa: BLE001 - normalized tool boundary
            return ToolResult(
                action.id,
                action.tool,
                "failed",
                error=f"{type(error).__name__}: {error}",
                duration_seconds=time.monotonic() - started,
            )
        finally:
            if future.done():
                executor.shutdown(wait=True, cancel_futures=True)


class AutonomousAgent:
    """Execute a bounded action graph and stop on blockers or budget limits."""

    def __init__(
        self,
        registry: ToolRegistry,
        *,
        max_rounds: int = 12,
        max_actions: int = 32,
        event_handler: EventHandler | None = None,
        checkpoint_handler: CheckpointHandler | None = None,
        cancel_handler: CancelHandler | None = None,
        replan_handler: ReplanHandler | None = None,
    ) -> None:
        self.registry = registry
        self.max_rounds = max(1, min(int(max_rounds), 100))
        self.max_actions = max(1, min(int(max_actions), 500))
        self.event_handler = event_handler or (lambda _kind, _detail, _payload: None)
        self.checkpoint_handler = checkpoint_handler or (lambda _checkpoint: None)
        self.cancel_handler = cancel_handler or (lambda: False)
        self.replan_handler = replan_handler

    def _emit(self, kind: str, detail: str, payload: dict[str, object] | None = None) -> None:
        self.event_handler(kind, detail, payload or {})

    def _checkpoint(self, run: AgentRun, actions: list[Action], pending: list[Action]) -> None:
        self.checkpoint_handler({
            **run.as_dict(),
            "actions": [item.as_dict() for item in actions],
            "pending_actions": [item.as_dict() for item in pending],
        })

    def run(
        self,
        target: str,
        actions: Iterable[Action],
        *,
        context: dict[str, object] | None = None,
        checkpoint: dict[str, object] | None = None,
    ) -> AgentRun:
        run = AgentRun(target=target)
        queue = list(actions)
        if checkpoint:
            stored_actions = checkpoint.get("actions", [])
            if isinstance(stored_actions, list) and stored_actions:
                queue = [Action.from_dict(item) for item in stored_actions if isinstance(item, dict)]
            run = AgentRun(
                target=target,
                status="running",
                stop_reason="",
                rounds=int(checkpoint.get("rounds", 0)),
                results=[ToolResult(**item) for item in checkpoint.get("results", []) if isinstance(item, dict)],
                started_at=float(checkpoint.get("started_at", time.time())),
                generation=int(checkpoint.get("generation", 0)),
            )
        if len(queue) > self.max_actions:
            queue = queue[: self.max_actions]
        ids = [item.id for item in queue]
        if not all(ids) or len(set(ids)) != len(ids):
            raise ValueError("autonomous actions require unique non-empty ids")
        known = set(ids)
        for action in queue:
            missing = set(action.depends_on) - known
            if missing or action.id in action.depends_on:
                raise ValueError(f"invalid dependencies for {action.id}: {sorted(missing)}")

        self._emit("agent.resumed" if checkpoint else "agent.started", target, {"actions": len(queue), "max_rounds": self.max_rounds, "generation": run.generation})
        outcomes: dict[str, ToolResult] = {
            item.action_id: item
            for item in run.results
            if item.status not in {"approval_required"}
        }
        stored_pending = checkpoint.get("pending_actions", []) if checkpoint else []
        pending = [Action.from_dict(item) for item in stored_pending if isinstance(item, dict)] if stored_pending else [item for item in queue if item.id not in outcomes]
        self._checkpoint(run, queue, pending)
        while pending and run.rounds < self.max_rounds:
            if self.cancel_handler():
                run.status = "cancelled"
                run.stop_reason = "operator cancellation requested"
                break
            ready = [item for item in pending if all(dep in outcomes for dep in item.depends_on)]
            if not ready:
                run.status = "blocked"
                run.stop_reason = "dependency cycle or unresolved prerequisite"
                break
            run.rounds += 1
            progressed = False
            for action in ready:
                if self.cancel_handler():
                    run.status = "cancelled"
                    run.stop_reason = "operator cancellation requested"
                    break
                failed = [dep for dep in action.depends_on if outcomes[dep].status != "completed"]
                if failed:
                    result = ToolResult(action.id, action.tool, "blocked", error=f"prerequisite did not complete: {failed}")
                else:
                    self._emit("tool.started", action.tool, {"action_id": action.id, "round": run.rounds})
                    result = self.registry.execute(action, target, context={**(context or {}), "observations": {key: value.observation for key, value in outcomes.items()}})
                outcomes[action.id] = result
                run.results = [item for item in run.results if not (item.action_id == action.id and item.status == "approval_required")]
                run.results.append(result)
                if result.status != "approval_required":
                    pending.remove(action)
                progressed = True
                self._emit(
                    "tool.completed" if result.status == "completed" else "tool.blocked" if result.status in {"blocked", "approval_required"} else "tool.failed",
                    action.tool,
                    {"action_id": action.id, "status": result.status, "approval_id": result.approval_id, "error": result.error},
                )
                if result.status == "approval_required":
                    self._emit(
                        "approval.requested",
                        result.approval_id,
                        {"action_id": action.id, "tool": action.tool, "target": target},
                    )
                    run.status = "waiting_approval"
                    run.stop_reason = f"approval required: {result.approval_id}"
                    self._checkpoint(run, queue, pending)
                    break
                if bool(result.observation.get("objective_met")):
                    run.status = "completed"
                    run.stop_reason = f"objective met by {action.id}"
                    pending.clear()
                    break
                self._checkpoint(run, queue, pending)
            if run.status in {"waiting_approval", "cancelled", "completed"}:
                break
            if self.replan_handler:
                additions = list(self.replan_handler(run, list(pending)))
                existing_ids = {item.id for item in queue}
                additions = [item for item in additions if item.id not in existing_ids]
                if additions:
                    if len(queue) + len(additions) > self.max_actions:
                        additions = additions[: max(0, self.max_actions - len(queue))]
                    allowed_ids = existing_ids | {item.id for item in additions}
                    for item in additions:
                        missing = set(item.depends_on) - allowed_ids
                        if not item.id or item.id in item.depends_on or missing:
                            raise ValueError(f"replanner emitted invalid action {item.id!r}: missing={sorted(missing)}")
                    queue.extend(additions)
                    pending.extend(additions)
                    run.generation += 1
                    self._emit("agent.replanned", str(run.generation), {"added": [item.id for item in additions]})
                    self._checkpoint(run, queue, pending)
            if not progressed:
                break

        if run.status == "running":
            if pending:
                run.status = "budget_exhausted"
                run.stop_reason = f"round budget exhausted ({self.max_rounds})"
            elif any(item.status == "failed" for item in run.results):
                run.status = "completed_with_errors"
                run.stop_reason = "all reachable actions processed"
            elif any(item.status == "blocked" for item in run.results):
                run.status = "completed_with_blocks"
                run.stop_reason = "all reachable actions processed"
            else:
                run.status = "completed"
                run.stop_reason = "all actions completed"
        run.finished_at = time.time()
        self._checkpoint(run, queue, pending)
        self._emit("agent.completed", run.status, {"rounds": run.rounds, "results": len(run.results), "reason": run.stop_reason})
        return run
