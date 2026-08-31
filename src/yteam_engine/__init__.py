"""YTEAM Engine — composable, policy-bound security engineering core.

This package layers durable orchestration on top of the standalone YTEAM
runtime. It never shells out to an upstream agent, never reads machine-specific
paths, and every component fails closed by default.

Components
----------
- graph:          DAG task-graph executor (topological order, deps, retry, policy).
- scheduler:      durable multi-target scheduler (priority, per-target rate, anti-thrash).
- planner:        adaptive recon/attack planner (self-tuning state machine).
- knowledge:      knowledge graph + verified lesson ledger with traversal queries.
- policy:         schema-validated, deny-by-default policy resolution.
- skill_resolver: plugin/skill runtime resolver (DI, LRU cache, load policy).
"""

from __future__ import annotations

import json
from pathlib import Path

from .graph import TaskGraph, TaskNode, NodeOutcome, GraphRun, run_graph_to_dict
from .knowledge import KnowledgeGraph, KnowledgeError
from .planner import AdaptivePlanner, PlannerState, StepResult, plan_to_dict
from .policy import Policy, PolicyError, PolicyViolation, ResolvedBudget
from .scheduler import Scheduler, Admission, TargetEntry
from .skill_resolver import SkillResolver, SkillResolutionError
from .context_guard import (
    ContextGuard,
    GuardConfig,
    GuardVerdict,
    estimate_conversation_tokens,
    estimate_message_tokens,
    estimate_tokens,
)

__all__ = [
    "AdaptivePlanner",
    "Admission",
    "ContextGuard",
    "Engine",
    "GraphRun",
    "GuardConfig",
    "GuardVerdict",
    "KnowledgeError",
    "KnowledgeGraph",
    "NodeOutcome",
    "PlannerState",
    "Policy",
    "PolicyError",
    "PolicyViolation",
    "ResolvedBudget",
    "Scheduler",
    "SkillResolutionError",
    "SkillResolver",
    "StepResult",
    "TargetEntry",
    "TaskGraph",
    "TaskNode",
    "build_default_policy",
    "default_policy_path",
    "engine_status",
    "estimate_conversation_tokens",
    "estimate_message_tokens",
    "estimate_tokens",
    "make_engine",
    "plan_to_dict",
    "run_graph_to_dict",
]


def default_policy_path(root: Path | None = None) -> Path:
    root = root or Path(__file__).resolve().parents[2]
    return root / "runtime" / "policy.json"


def build_default_policy(path: Path | None = None) -> Policy:
    """Load a policy file if present, else a strict read-only default."""
    path = path or default_policy_path()
    if path.exists():
        return Policy.from_file(path)
    return Policy.default()


class Engine:
    """Facade bundling scheduler, planner, knowledge, policy, and resolver."""

    def __init__(self, root: Path, policy: Policy | None = None, max_workers: int = 2) -> None:
        self.root = Path(root)
        self.runtime_dir = self.root / "runtime"
        self.policy = policy or build_default_policy(self.runtime_dir / "policy.json")
        self.scheduler = Scheduler(self.policy, store=None, max_workers=max_workers)
        self.planner = AdaptivePlanner(self.policy)
        self.knowledge = KnowledgeGraph(self.runtime_dir / "knowledge" / "graph.jsonl")
        self.skills = SkillResolver(
            policy=self.policy,
            registry_fn=self._registry,
            get_fn=self._get_skill,
            # First-party YTEAM skills are reviewed safe playbooks; load their
            # bodies for reasoning. Quarantined content is still refused.
            allow_controlled=True,
        )

    @staticmethod
    def _registry() -> list[dict[str, object]]:
        from yteam_skills import registry

        return registry()

    @staticmethod
    def _get_skill(items: list[dict[str, object]], name: str, section: str = "", allow_controlled: bool = False) -> dict[str, object]:
        from yteam_skills import get_skill

        return get_skill(items, name, section, allow_controlled)

    def snapshot(self) -> dict[str, object]:
        return {
            "policy": self.policy.to_dict(),
            "scheduler": self.scheduler.summary(),
            "planner_weights": {k: round(v, 3) for k, v in self.planner.weights().items()},
            "knowledge": self.knowledge.summary(),
            "skill_cache": self.skills.cache_stats(),
        }


def make_engine(root: Path | None = None, policy: Policy | None = None) -> Engine:
    root = root or Path(__file__).resolve().parents[2]
    return Engine(root, policy)


def engine_status(root: Path | None = None) -> str:
    engine = make_engine(root)
    return json.dumps(engine.snapshot(), indent=2)
