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

    def test_failed_prerequisite_blocks_dependent_without_invoke(self) -> None:
        from yteam_engine.graph import TaskGraph, TaskNode
        from yteam_engine.policy import Policy

        invoked = {"dependent": False}

        def bad(ctx):
            raise RuntimeError("boom")

        def dependent(ctx):
            invoked["dependent"] = True
            return {"v": 1}

        graph = TaskGraph(Policy.default())
        graph.add(TaskNode(id="bad", fn=bad))
        graph.add(TaskNode(id="dependent", fn=dependent, deps=["bad"]))
        run = graph.run()
        self.assertEqual(run.failed, 1)
        self.assertEqual(run.blocked, 1)
        self.assertEqual(run.by_id()["dependent"].status, "blocked")
        self.assertIn("bad", run.by_id()["dependent"].error)
        self.assertFalse(invoked["dependent"])

    def test_graph_policy_target_scoped(self) -> None:
        from yteam_engine.graph import TaskGraph, TaskNode
        from yteam_engine.policy import Policy, PolicyViolation

        policy = Policy({
            "schema_version": 1,
            "default_side_effect": "read",
            "per_target_rate": 1.0,
            "targets": {"api.example.com": {"allowed_effects": ["read", "analyze"]}},
        })
        graph = TaskGraph(policy, target="api.example.com")
        graph.add(TaskNode(id="analysis", fn=lambda ctx: {"ok": True}, side_effect="analyze"))
        run = graph.run()
        self.assertEqual(run.completed, 1)

    def test_scheduler_rate_update_reflects_in_bucket(self) -> None:
        from yteam_engine.policy import Policy
        from yteam_engine.scheduler import Scheduler

        scheduler = Scheduler(Policy.default())
        scheduler.register("api.example.com", rate_per_second=5.0)
        scheduler.register("api.example.com", rate_per_second=9.0)
        self.assertEqual(scheduler._buckets["api.example.com"].rate, 9.0)


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


class EngineAutonomyTests(unittest.TestCase):
    def test_agent_executes_dependencies_and_stops_on_objective(self) -> None:
        from yteam_engine import Action, AutonomousAgent, ToolRegistry, ToolSpec
        from yteam_engine.policy import Policy

        calls: list[str] = []
        events: list[str] = []
        registry = ToolRegistry(Policy.default())
        registry.register(ToolSpec("scope.validate", "scope", "read", lambda payload: calls.append("scope") or {"allowed": True}))
        registry.register(ToolSpec("artifact.analyze", "analyze", "read", lambda payload: calls.append("analyze") or {"objective_met": True, "signal": True}))
        registry.register(ToolSpec("unused", "unused", "read", lambda payload: calls.append("unused") or {"ok": True}))
        agent = AutonomousAgent(registry, event_handler=lambda kind, detail, payload: events.append(kind))
        run = agent.run("https://example.com", [
            Action("scope", "scope.validate"),
            Action("analyze", "artifact.analyze", depends_on=("scope",)),
            Action("unused", "unused", depends_on=("analyze",)),
        ])
        self.assertEqual(run.status, "completed")
        self.assertEqual(calls, ["scope", "analyze"])
        self.assertIn("objective met", run.stop_reason)
        self.assertIn("agent.started", events)
        self.assertIn("agent.completed", events)

    def test_unknown_and_policy_denied_tools_fail_closed(self) -> None:
        from yteam_engine import Action, ToolRegistry, ToolSpec
        from yteam_engine.policy import Policy

        registry = ToolRegistry(Policy.default())
        registry.register(ToolSpec("writer", "write", "write", lambda payload: {"ok": True}))
        unknown = registry.execute(Action("a", "missing"), "example.com")
        denied = registry.execute(Action("b", "writer"), "example.com")
        self.assertEqual(unknown.status, "blocked")
        self.assertEqual(denied.status, "blocked")
        self.assertIn("not registered", unknown.error)
        self.assertIn("not allowed", denied.error)

    def test_durable_approval_required_then_accepted(self) -> None:
        from yteam_engine import Action, ToolRegistry, ToolSpec
        from yteam_engine.policy import Policy
        from yteam_state import StateStore

        policy = Policy({
            "schema_version": 1,
            "default_side_effect": "write",
            "per_target_rate": 1.0,
            "targets": {},
        })
        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state.db")
            registry = ToolRegistry(policy, approval_store=store)
            registry.register(ToolSpec("reviewed.write", "reviewed local action", "write", lambda payload: {"ok": True}, requires_approval=True))
            action = Action("write", "reviewed.write", {"value": "safe"})
            waiting = registry.execute(action, "example.com")
            self.assertEqual(waiting.status, "approval_required")
            self.assertTrue(waiting.approval_id.startswith("apr_"))
            store.resolve_approval(waiting.approval_id, "approved")
            completed = registry.execute(action, "example.com", approval_id=waiting.approval_id)
            self.assertEqual(completed.status, "completed")
            self.assertTrue(completed.observation["ok"])

    def test_tool_output_is_bounded(self) -> None:
        from yteam_engine import Action, ToolRegistry, ToolSpec
        from yteam_engine.policy import Policy

        registry = ToolRegistry(Policy.default())
        registry.register(ToolSpec("large", "large", "read", lambda payload: {"body": "x" * 10000}, max_output_bytes=512))
        result = registry.execute(Action("large", "large"), "example.com")
        self.assertEqual(result.status, "completed")
        self.assertTrue(result.observation["truncated"])
        self.assertIn("sha256", result.observation)

    def test_invalid_action_graph_is_rejected(self) -> None:
        from yteam_engine import Action, AutonomousAgent, ToolRegistry
        from yteam_engine.policy import Policy

        agent = AutonomousAgent(ToolRegistry(Policy.default()))
        with self.assertRaises(ValueError):
            agent.run("example.com", [Action("a", "x", depends_on=("missing",))])

    def test_observation_replanning_adds_and_executes_actions(self) -> None:
        from yteam_engine import Action, AutonomousAgent, ToolRegistry, ToolSpec
        from yteam_engine.policy import Policy

        calls: list[str] = []
        registry = ToolRegistry(Policy.default())
        registry.register(ToolSpec("observe", "observe", "read", lambda payload: calls.append("observe") or {"signal": True}))
        registry.register(ToolSpec("review", "review", "read", lambda payload: calls.append("review") or {"objective_met": True}))

        def replan(run, pending):
            if any(item.action_id == "observe" and item.status == "completed" for item in run.results):
                return [Action("review", "review", depends_on=("observe",))]
            return []

        agent = AutonomousAgent(registry, replan_handler=replan)
        run = agent.run("example.com", [Action("observe", "observe")])
        self.assertEqual(run.status, "completed")
        self.assertEqual(run.generation, 1)
        self.assertEqual(calls, ["observe", "review"])

    def test_approval_checkpoint_resumes_exact_action_once(self) -> None:
        from yteam_engine import Action, AutonomousAgent, ToolRegistry, ToolSpec
        from yteam_engine.policy import Policy
        from yteam_state import StateStore

        policy = Policy({"schema_version": 1, "default_side_effect": "write", "per_target_rate": 1.0, "targets": {}})
        checkpoints: list[dict[str, object]] = []
        calls = {"count": 0}
        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state.db")
            registry = ToolRegistry(policy, approval_store=store)
            registry.register(ToolSpec("reviewed", "reviewed", "write", lambda payload: calls.__setitem__("count", calls["count"] + 1) or {"objective_met": True}, requires_approval=True))
            first = AutonomousAgent(registry, checkpoint_handler=checkpoints.append).run(
                "example.com",
                [Action("risk", "reviewed")],
                context={"job_id": "job-1"},
            )
            self.assertEqual(first.status, "waiting_approval")
            approval = store.approval_for_action("job-1", "risk")
            self.assertIsNotNone(approval)
            store.resolve_approval(str(approval["id"]), "approved")
            second = AutonomousAgent(registry, checkpoint_handler=checkpoints.append).run(
                "example.com",
                [Action("risk", "reviewed")],
                context={"job_id": "job-1"},
                checkpoint=checkpoints[-1],
            )
            self.assertEqual(second.status, "completed")
            self.assertEqual(calls["count"], 1)
            self.assertGreater(store.approval(str(approval["id"]))["consumed_at"], 0)

    def test_cancel_handler_stops_before_next_action(self) -> None:
        from yteam_engine import Action, AutonomousAgent, ToolRegistry, ToolSpec
        from yteam_engine.policy import Policy

        registry = ToolRegistry(Policy.default())
        registry.register(ToolSpec("safe", "safe", "read", lambda payload: {"ok": True}))
        run = AutonomousAgent(registry, cancel_handler=lambda: True).run("example.com", [Action("safe", "safe")])
        self.assertEqual(run.status, "cancelled")
        self.assertEqual(run.results, [])


class EngineContextGuardTests(unittest.TestCase):
    def _messages(self, count: int) -> list[dict[str, str]]:
        return [
            {"role": "user" if i % 2 == 0 else "assistant", "content": f"Turn number {i} with some reasonably long conversational content to estimate tokens for the context guard window management across a long session."}
            for i in range(count)
        ]

    def test_estimate_is_deterministic(self) -> None:
        from yteam_engine.context_guard import estimate_tokens

        text = "hello world this is a test"
        self.assertEqual(estimate_tokens(text), estimate_tokens(text))
        self.assertGreater(estimate_tokens(text), 0)

    def test_ok_below_warning(self) -> None:
        from yteam_engine.context_guard import ContextGuard, GuardConfig

        guard = ContextGuard(GuardConfig(context_window=1_000_000))
        verdict = guard.check(self._messages(10))
        self.assertEqual(verdict.level, "ok")
        self.assertFalse(verdict.compaction_applied)

    def test_compaction_at_warning_threshold(self) -> None:
        from yteam_engine.context_guard import ContextGuard, GuardConfig

        guard = ContextGuard(GuardConfig(context_window=2000, warning_ratio=0.5, handoff_ratio=0.9, keep_recent_messages=4))
        messages = self._messages(40)
        verdict = guard.check(messages)
        self.assertEqual(verdict.level, "warning")
        self.assertTrue(verdict.compaction_applied)
        self.assertGreater(verdict.compaction_saved_tokens, 0)
        compacted, _ = guard.compact(messages)
        self.assertLessEqual(len(compacted), 5)  # 1 summary + 4 recent

    def test_handoff_at_critical_threshold(self) -> None:
        import tempfile

        from yteam_engine.context_guard import ContextGuard, GuardConfig

        with tempfile.TemporaryDirectory() as directory:
            guard = ContextGuard(GuardConfig(context_window=1500, warning_ratio=0.2, handoff_ratio=0.3), handoff_dir=Path(directory) / "handoffs")
            verdict = guard.check(self._messages(40))
            self.assertEqual(verdict.level, "handoff")
            self.assertTrue(verdict.handoff_path)
            self.assertTrue(Path(verdict.handoff_path).exists())
            self.assertIn("yteam --handoff", verdict.continue_cmd)

    def test_runtime_ctx_command(self) -> None:
        import tempfile

        from yteam_runtime import YteamRuntime

        with tempfile.TemporaryDirectory() as directory:
            with patch("yteam_runtime.discover_free_models", return_value=["model-test"]):
                runtime = YteamRuntime(Path(directory))
            output = runtime.command("/ctx")
            self.assertIn("level", output)


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
