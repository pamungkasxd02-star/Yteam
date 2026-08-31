"""DAG task-graph executor for YTEAM pipelines.

A pipeline is a directed acyclic graph of task nodes. Each node declares its
dependencies, an allowed method, an optional retry policy, and a side-effect
class. The executor runs nodes in topological order with a bounded concurrency
budget, propagates outcomes into a shared ledger, and refuses to run any node
that violates the bound policy.

This is intentionally generic so both the adaptive planner and the durable
scheduler can reuse it.
"""

from __future__ import annotations

import json
import threading
from dataclasses import dataclass, field
from typing import Callable, Iterable

from .policy import Policy, PolicyViolation

SideEffect = str  # one of "read", "write", "analyze", "external", "report"


@dataclass
class TaskNode:
    """A single unit of work in a pipeline DAG."""

    id: str
    fn: Callable[[dict[str, object]], dict[str, object]]
    deps: list[str] = field(default_factory=list)
    side_effect: SideEffect = "read"
    max_attempts: int = 1
    backoff_seconds: float = 1.0
    metadata: dict[str, object] = field(default_factory=dict)

    def validate(self) -> None:
        if not self.id or not str(self.id).strip():
            raise ValueError("task node requires a non-empty id")
        if not callable(self.fn):
            raise TypeError(f"task {self.id!r} fn must be callable")
        if self.side_effect not in {"read", "write", "analyze", "external", "report"}:
            raise ValueError(f"task {self.id!r} has unknown side_effect {self.side_effect!r}")
        if self.max_attempts < 1:
            raise ValueError(f"task {self.id!r} max_attempts must be >= 1")


@dataclass
class NodeOutcome:
    node_id: str
    status: str  # "completed", "failed", "blocked"
    attempts: int
    error: str = ""
    result: dict[str, object] = field(default_factory=dict)
    duration_seconds: float = 0.0


@dataclass
class GraphRun:
    outcomes: list[NodeOutcome] = field(default_factory=list)
    completed: int = 0
    failed: int = 0
    blocked: int = 0

    def by_id(self) -> dict[str, NodeOutcome]:
        return {item.node_id: item for item in self.outcomes}

    def succeeded(self, node_id: str) -> bool:
        outcome = self.by_id().get(node_id)
        return bool(outcome and outcome.status == "completed")


class TaskGraph:
    """A policy-bound DAG of tasks with bounded parallel execution."""

    def __init__(self, policy: Policy, max_workers: int = 4, target: str = "") -> None:
        self.policy = policy
        self.max_workers = max(1, int(max_workers))
        self.target = target
        self.nodes: dict[str, TaskNode] = {}
        self._lock = threading.RLock()

    def add(self, node: TaskNode) -> "TaskGraph":
        node.validate()
        with self._lock:
            if node.id in self.nodes:
                raise ValueError(f"duplicate task node id: {node.id}")
            for dep in node.deps:
                if dep == node.id:
                    raise ValueError(f"task {node.id!r} cannot depend on itself")
            self.nodes[node.id] = node
        return self

    def add_many(self, nodes: Iterable[TaskNode]) -> "TaskGraph":
        for node in nodes:
            self.add(node)
        return self

    def _topological_order(self) -> list[str]:
        pending = set(self.nodes)
        order: list[str] = []
        while pending:
            ready = [
                node_id
                for node_id in pending
                if all(dep not in pending for dep in self.nodes[node_id].deps)
            ]
            if not ready:
                cycle = sorted(pending)
                raise ValueError(f"task graph contains a dependency cycle among: {cycle}")
            order.extend(sorted(ready))
            pending.difference_update(ready)
        return order

    def _resolve(self, node: TaskNode, results: dict[str, dict[str, object]]) -> dict[str, object]:
        # Policy gate: does the declared side-effect class fit the active policy?
        self.policy.assert_effect(node.side_effect, self.target or f"task:{node.id}")
        resolved: dict[str, object] = {"self": dict(node.metadata)}
        for dep in node.deps:
            resolved[dep] = results[dep]
        return resolved

    def run(self, context: dict[str, object] | None = None) -> GraphRun:
        """Execute the graph, returning per-node outcomes."""
        self._validate_dependencies_exist()
        order = self._topological_order()
        run = GraphRun()
        results: dict[str, dict[str, object]] = {}
        shared = dict(context or {})

        def execute(node_id: str) -> NodeOutcome:
            node = self.nodes[node_id]
            outcome = NodeOutcome(node_id=node_id, status="failed", attempts=0)
            import time
            total_started = time.monotonic()

            for attempt in range(1, node.max_attempts + 1):
                outcome.attempts = attempt
                started = time.monotonic()
                try:
                    inputs = self._resolve(node, results)
                    payload = {**inputs, **shared}
                    result = node.fn(payload)
                    if not isinstance(result, dict):
                        raise TypeError(f"task {node_id!r} returned non-dict: {type(result).__name__}")
                    with self._lock:
                        results[node_id] = result
                    outcome.status = "completed"
                    outcome.result = result
                    outcome.duration_seconds = time.monotonic() - started
                    return outcome
                except (PolicyViolation,) as error:
                    outcome.status = "blocked"
                    outcome.error = str(error)
                    outcome.duration_seconds = time.monotonic() - started
                    return outcome
                except Exception as error:  # noqa: BLE001
                    outcome.error = f"{type(error).__name__}: {error}"
                    if attempt < node.max_attempts:
                        time.sleep(node.backoff_seconds * attempt)
            outcome.duration_seconds = time.monotonic() - total_started
            return outcome

        from concurrent.futures import ThreadPoolExecutor

        pending = set(self.nodes)
        outcomes: dict[str, NodeOutcome] = {}
        while pending:
            ready = sorted(node_id for node_id in pending if all(dep in outcomes for dep in self.nodes[node_id].deps))
            if not ready:
                raise ValueError(f"task graph contains a dependency cycle among: {sorted(pending)}")
            batch: list[tuple[str, NodeOutcome]] = []
            with ThreadPoolExecutor(max_workers=self.max_workers) as executor:
                futures = {}
                for node_id in ready:
                    failed_deps = [dep for dep in self.nodes[node_id].deps if outcomes[dep].status != "completed"]
                    if failed_deps:
                        futures[node_id] = None
                        batch.append((node_id, NodeOutcome(node_id, "blocked", 0, f"dependency did not complete: {failed_deps}")))
                    else:
                        futures[node_id] = executor.submit(execute, node_id)
                for node_id, future in futures.items():
                    if future is not None:
                        batch.append((node_id, future.result()))
            for node_id, outcome in sorted(batch, key=lambda item: item[0]):
                pending.remove(node_id)
                outcomes[node_id] = outcome
                run.outcomes.append(outcome)
                if outcome.status == "completed":
                    run.completed += 1
                    results[node_id] = outcome.result
                elif outcome.status == "blocked":
                    run.blocked += 1
                else:
                    run.failed += 1

        run.outcomes.sort(key=lambda item: item.node_id)
        return run

    def _validate_dependencies_exist(self) -> None:
        with self._lock:
            for node_id, node in self.nodes.items():
                for dep in node.deps:
                    if dep not in self.nodes:
                        raise ValueError(f"task {node_id!r} depends on unknown task {dep!r}")

    def to_dict(self) -> dict[str, object]:
        return {
            "node_count": len(self.nodes),
            "max_workers": self.max_workers,
            "target": self.target,
            "nodes": [
                {
                    "id": node.id,
                    "deps": list(node.deps),
                    "side_effect": node.side_effect,
                    "max_attempts": node.max_attempts,
                    "metadata": node.metadata,
                }
                for node in sorted(self.nodes.values(), key=lambda item: item.id)
            ],
        }


def run_graph_to_dict(run: GraphRun) -> dict[str, object]:
    return {
        "completed": run.completed,
        "failed": run.failed,
        "blocked": run.blocked,
        "outcomes": [vars(item) for item in run.outcomes],
    }
