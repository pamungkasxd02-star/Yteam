from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "scripts"
SRC = ROOT / "src"
for entry in (SCRIPTS, SRC):
    if str(entry) not in sys.path:
        sys.path.insert(0, str(entry))


POLICY = {
    "schema_version": 1,
    "default_side_effect": "read",
    "per_target_rate": 1.0,
    "targets": {
        "api.example.com": {"allowed_effects": ["read", "analyze"], "rate_per_second": 2.0},
        "*.example.com": {"rate_per_second": 0.5},
    },
}


class EnginePolicyTests(unittest.TestCase):
    def test_default_is_read_only_deny_by_default(self) -> None:
        from yteam_engine.policy import Policy

        policy = Policy.default()
        self.assertEqual(policy.default_effect, "read")
        self.assertRaises(Exception, policy.assert_effect, "write", "https://anything.example")
        # read is allowed by default
        policy.assert_effect("read", "https://anything.example")

    def test_target_specific_budget_and_deny(self) -> None:
        from yteam_engine.policy import Policy, PolicyViolation

        policy = Policy(POLICY)
        budget = policy.budget_for("api.example.com")
        self.assertIn("analyze", budget.allowed_effects)
        self.assertEqual(budget.rate_per_second, 2.0)
        policy.assert_effect("analyze", "api.example.com")
        with self.assertRaises(PolicyViolation):
            policy.assert_effect("write", "api.example.com")

    def test_schema_validation_rejects_unknown_keys(self) -> None:
        from yteam_engine.policy import Policy, PolicyError

        with self.assertRaises(PolicyError):
            Policy({"schema_version": 1, "nope": True})


class EngineGraphTests(unittest.TestCase):
    def test_dag_runs_in_order_and_passes_dependencies(self) -> None:
        from yteam_engine.graph import TaskGraph, TaskNode
        from yteam_engine.policy import Policy

        graph = TaskGraph(Policy.default())
        graph.add(TaskNode(id="a", fn=lambda ctx: {"v": 1}))
        graph.add(TaskNode(id="b", fn=lambda ctx: {"v": ctx["a"]["v"] + 1}, deps=["a"]))
        graph.add(TaskNode(id="c", fn=lambda ctx: {"v": ctx["b"]["v"] * 10}, deps=["b"]))
        run = graph.run()
        self.assertEqual(run.completed, 3)
        self.assertEqual(run.failed, 0)
        self.assertTrue(run.succeeded("c"))
        self.assertEqual(run.by_id()["c"].result["v"], 20)

    def test_cycle_detected(self) -> None:
        from yteam_engine.graph import TaskGraph, TaskNode
        from yteam_engine.policy import Policy

        graph = TaskGraph(Policy.default())
        graph.add(TaskNode(id="a", fn=lambda ctx: {"v": 1}, deps=["b"]))
        graph.add(TaskNode(id="b", fn=lambda ctx: {"v": 2}, deps=["a"]))
        with self.assertRaises(ValueError):
            graph.run()

    def test_policy_blocks_write_effect(self) -> None:
        from yteam_engine.graph import TaskGraph, TaskNode
        from yteam_engine.policy import Policy

        graph = TaskGraph(Policy.default())
        graph.add(TaskNode(id="w", fn=lambda ctx: {"v": 1}, side_effect="write"))
        run = graph.run()
        self.assertEqual(run.blocked, 1)
        self.assertEqual(run.outcomes[0].status, "blocked")

    def test_retry_on_failure(self) -> None:
        from yteam_engine.graph import TaskGraph, TaskNode
        from yteam_engine.policy import Policy

        state = {"calls": 0}

        def flaky(ctx):
            state["calls"] += 1
            if state["calls"] < 3:
                raise RuntimeError("transient")
            return {"ok": True}

        graph = TaskGraph(Policy.default())
        graph.add(TaskNode(id="f", fn=flaky, max_attempts=3, backoff_seconds=0.0))
        run = graph.run()
        self.assertEqual(run.completed, 1)
        self.assertEqual(state["calls"], 3)


class EngineSchedulerTests(unittest.TestCase):
    def test_admit_and_policy_gate(self) -> None:
        from yteam_engine.policy import Policy, PolicyViolation
        from yteam_engine.scheduler import Scheduler

        scheduler = Scheduler(Policy(POLICY), max_workers=2)
        admitted = scheduler.admit("api.example.com", params={"depth": 2})
        self.assertTrue(admitted.admitted)
        self.assertEqual(scheduler.summary()["target_count"], 1)
        # a target with no matching rule still admits read under deny-by-default
        admitted2 = scheduler.admit("other.unknown.example", params={})
        self.assertTrue(admitted2.admitted)
        # but a write effect is denied at the policy budget level
        policy = Policy(POLICY)
        with self.assertRaises(PolicyViolation):
            policy.assert_effect("write", "api.example.com")

    def test_anti_thrash_cool_down(self) -> None:
        from yteam_engine.policy import Policy
        from yteam_engine.scheduler import Scheduler

        scheduler = Scheduler(Policy.default(), cool_down_seconds=60)
        scheduler.admit("api.example.com")
        scheduler.complete("api.example.com", success=True)
        re_admit = scheduler.admit("api.example.com")
        self.assertFalse(re_admit.admitted)
        self.assertIn("cool-down", re_admit.reason)

    def test_rate_bucket_throttles(self) -> None:
        from yteam_engine.policy import Policy
        from yteam_engine.scheduler import Scheduler, TokenBucket

        bucket = TokenBucket(rate_per_second=1.0, burst=1)
        self.assertTrue(bucket.acquire())
        self.assertFalse(bucket.acquire())
        self.assertGreater(bucket.next_available(), 0)


class EnginePlannerTests(unittest.TestCase):
    def test_plan_respects_policy_and_adapts(self) -> None:
        from yteam_engine.planner import AdaptivePlanner, PlannerState, StepResult
        from yteam_engine.policy import Policy

        planner = AdaptivePlanner(Policy(POLICY))
        state = PlannerState(target="api.example.com")
        plan = planner.plan(state)
        self.assertTrue(plan)
        # all planned techniques are read/analyze (allowed), never write/inject
        for step in plan:
            self.assertIn(step.side_effect, {"read", "analyze"})
        state = planner.apply_result(state, StepResult(technique="recon.js_bundle", reward=1.0))
        state = planner.apply_result(state, StepResult(technique="data.idor_read", reward=-1.0))
        weights = planner.weights()
        self.assertGreater(weights["recon"], 1.0)
        self.assertLess(weights["data"], 0.7)


class EngineKnowledgeTests(unittest.TestCase):
    def test_graph_add_query_and_traverse(self) -> None:
        from yteam_engine.knowledge import KnowledgeGraph

        with tempfile.TemporaryDirectory() as directory:
            graph = KnowledgeGraph(Path(directory) / "graph.jsonl")
            target = graph.add_node("target", "api.example.com")
            finding = graph.add_node("finding", "IDOR read on /v1/users/{id}", {"severity": "high"})
            graph.add_edge(target["id"], "observed_on", finding["id"])
            self.assertEqual(graph.summary()["nodes"], 2)
            self.assertEqual(graph.summary()["edges"], 1)
            self.assertGreaterEqual(len(graph.neighbors(target["id"])), 1)
            # query filters
            self.assertEqual(len(graph.nodes(kind="finding", query="idor")), 1)
            self.assertEqual(len(graph.nodes(kind="finding", query="nosuch")), 0)

    def test_invalid_kind_rejected(self) -> None:
        from yteam_engine.knowledge import KnowledgeError, KnowledgeGraph

        with tempfile.TemporaryDirectory() as directory:
            graph = KnowledgeGraph(Path(directory) / "g.jsonl")
            with self.assertRaises(KnowledgeError):
                graph.add_node("nonsense", "x")


class EngineSkillResolverTests(unittest.TestCase):
    def test_resolver_loads_and_caches_first_party_skill(self) -> None:
        from yteam_engine.policy import Policy
        from yteam_engine.skill_resolver import SkillResolver

        def registry():
            from yteam_skills import registry as r

            return r()

        def get_fn(items, name, section="", allow=False):
            from yteam_skills import get_skill

            return get_skill(items, name, section, allow)

        resolver = SkillResolver(Policy.default(), registry, get_fn, cache_size=8, allow_controlled=True)
        loaded = resolver.resolve("yteam-recon")
        self.assertEqual(loaded["access"], "loaded")
        self.assertIn("Recon is complete", loaded["content"])
        stats = resolver.cache_stats()
        self.assertEqual(stats["size"], 1)

    def test_quarantined_skill_blocked(self) -> None:
        from yteam_engine.policy import Policy
        from yteam_engine.skill_resolver import SkillResolutionError, SkillResolver

        def registry():
            from yteam_skills import registry as r

            return r() + [{
                "name": "evil-shell",
                "description": "reverse-shell",
                "path": "",
                "source": "skills",
                "categories": ["injection"],
                "content_sha256": "x",
                "size_bytes": 0,
                "line_count": 0,
                "sections": [],
                "risk": "quarantined",
                "load_policy": "metadata_only",
            }]

        def get_fn(items, name, section="", allow=False):
            return {"name": name, "access": "quarantined", "content": ""}

        resolver = SkillResolver(Policy.default(), registry, get_fn)
        with self.assertRaises(SkillResolutionError):
            resolver.resolve("evil-shell")


class EngineBridgeTests(unittest.TestCase):
    def test_make_engine_and_status(self) -> None:
        from yteam_engine import Engine, make_engine

        with tempfile.TemporaryDirectory() as directory:
            engine = make_engine(Path(directory))
            snapshot = engine.snapshot()
            self.assertIn("policy", snapshot)
            self.assertIn("scheduler", snapshot)
            self.assertIn("planner_weights", snapshot)
            self.assertIn("knowledge", snapshot)
            self.assertIn("skill_cache", snapshot)
            # skill resolver is bound to the real first-party registry
            loaded = engine.skills.resolve("yteam-recon")
            self.assertEqual(loaded["access"], "loaded")


if __name__ == "__main__":
    unittest.main()
