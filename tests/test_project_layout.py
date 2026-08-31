from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[1]
SCRIPTS = ROOT / "scripts"
sys.path.insert(0, str(SCRIPTS))


class FixtureHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/":
            body = b"<html><title>YTEAM Fixture</title><a href='/api/profile?id=1'>profile</a><script src='/assets/app.js'></script></html>"
            status = 200
            content_type = "text/html"
        elif self.path == "/assets/app.js":
            body = b"fetch('/api/account?id=1')"
            status = 200
            content_type = "application/javascript"
        else:
            body = b"not found"
            status = 404
            content_type = "text/plain"
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args: object) -> None:
        return


class StandaloneYteamTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), FixtureHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        if str(SCRIPTS) in sys.path:
            sys.path.remove(str(SCRIPTS))

    def target(self) -> str:
        return f"http://127.0.0.1:{self.server.server_port}"

    def test_native_surface_has_no_required_upstream_runtime(self) -> None:
        required = (
            "yteam_tui.py", "yteam_runtime.py", "yteam_models.py", "yteam_ai.py",
            "yteam_session.py", "yteam_native_tools.py", "yteam_hunt.py", "yteam_worker.py",
        )
        for name in required:
            self.assertTrue((SCRIPTS / name).exists(), name)
        self.assertFalse((ROOT / "opencode.json").exists())
        self.assertFalse((SCRIPTS / "hermes_opencode.py").exists())
        self.assertFalse((SCRIPTS / "bootstrap_sources.py").exists())

    def test_superseded_wrappers_are_removed(self) -> None:
        for name in ("botterdop.py", "cybermes.py", "index_skills.py", "yteam_knowledge.py", "yteam_parallel.py", "yteam_run.py", "yteam_engine.py", "init_yteam.py", "yteam_assessment.py"):
            self.assertFalse((SCRIPTS / name).exists(), name)
        for relative in ("src/core/assessment.py", "src/decrypt/detect.py", "src/pentest_qa/qa.py", "src/server_guard/guard.py"):
            self.assertFalse((ROOT / relative).exists(), relative)
        self.assertTrue((ROOT / "src" / "local_solver" / "camoufox_adapter.py").exists())

    def test_profile_soul_is_loaded_as_native_system_prompt(self) -> None:
        from yteam_runtime import YteamRuntime

        with patch("yteam_runtime.discover_free_models", return_value=["model-a"]):
            runtime = YteamRuntime(ROOT)
        self.assertIn("YTEAM", runtime.profile_prompt)
        self.assertIn("authorized", runtime.profile_prompt.lower())

    def test_profile_and_docs_describe_standalone_runtime(self) -> None:
        config = (ROOT / "profile" / "config.yaml").read_text(encoding="utf-8")
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        self.assertIn("mode: standalone", config)
        self.assertIn("/bb <target>", readme)
        self.assertIn("does not require Bun", readme)
        self.assertIn("YTEAM Requirements", (ROOT / "REQUIREMENTS.md").read_text(encoding="utf-8"))

    def test_no_legacy_runtime_references_in_executable_source(self) -> None:
        forbidden = ("hermes_opencode", "bootstrap_sources", "run_cybermes_mcp", "vendor/cybermes", "vendor/hermes-agent")
        for path in SCRIPTS.glob("*.py"):
            text = path.read_text(encoding="utf-8")
            for marker in forbidden:
                self.assertNotIn(marker, text, f"{marker} in {path.name}")

    def test_model_config_defaults_and_local_override_are_safe(self) -> None:
        from yteam_models import DEFAULT_MODEL_CONFIG, load_model_config

        self.assertEqual(DEFAULT_MODEL_CONFIG["provider"], "zen-free")
        self.assertEqual(load_model_config(ROOT)["base_url"], "https://opencode.ai/zen/v1")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "yteam.local.yaml").write_text(
                "provider: test-provider\nmodel: test-model\napi_key: local-secret\nbase_url: https://model.test/v1\n",
                encoding="utf-8",
            )
            config = load_model_config(root)
            self.assertEqual(config["model"], "test-model")
            self.assertEqual(config["api_key"], "local-secret")

    def test_free_catalog_falls_back_when_network_is_unavailable(self) -> None:
        from yteam_models import FREE_MODEL_FALLBACK, discover_free_models

        with patch("yteam_models.urllib.request.urlopen", side_effect=OSError("offline")):
            self.assertEqual(discover_free_models(timeout=0.01), list(FREE_MODEL_FALLBACK))

    def test_session_store_is_bounded_and_jsonl(self) -> None:
        from yteam_session import Session

        with tempfile.TemporaryDirectory() as directory:
            session = Session(Path(directory), "test-session")
            for index in range(5):
                session.append("user", f"message-{index}")
            self.assertEqual(len(session.conversation(3)), 3)
            self.assertTrue((Path(directory) / "state.db").exists())
            reloaded = Session(Path(directory), "test-session")
            self.assertEqual(reloaded.messages[-1]["content"], "message-4")

    def test_runtime_routes_commands_and_records_events(self) -> None:
        from yteam_runtime import YteamRuntime

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "scripts").mkdir()
            for name in ("yteam_doctor.py",):
                (root / "scripts" / name).write_text("", encoding="utf-8")
            with patch("yteam_runtime.discover_free_models", return_value=["model-a", "model-b"]):
                runtime = YteamRuntime(root)
            self.assertIn("/bb", runtime.help_text())
            self.assertIn("model-a", runtime.models_text())
            self.assertIn("Active model: model-b", runtime.command("/model model-b"))
            self.assertIn("Queued durable read-only", runtime.command("/bb https://authorized.test"))
            self.assertEqual(runtime.state.list_jobs(limit=1)[0]["target"], "https://authorized.test")
            self.assertIn("Queued autonomous assessment", runtime.command("/auto https://authorized.test"))
            auto_job = runtime.state.list_jobs(limit=1)[0]
            self.assertEqual(auto_job["kind"], "autonomous_assessment")
            runtime.state.save_agent_checkpoint(auto_job["id"], auto_job["target"], "running", {"rounds": 1, "generation": 1, "pending_actions": []})
            self.assertIn(auto_job["id"], runtime.command("/agents"))
            self.assertIn("Cancelled", runtime.command(f"/cancel {auto_job['id']}"))
            approval = runtime.state.create_approval("reviewed.tool", "https://authorized.test", "test", {})
            self.assertIn(approval["id"], runtime.command("/approvals"))
            self.assertIn("is now approved", runtime.command(f"/approve {approval['id']}"))
            self.assertIn("Goodbye", runtime.command("/quit"))
            self.assertTrue(runtime.quit_requested)
            events = (root / "runtime" / "events.jsonl").read_text(encoding="utf-8")
            self.assertIn("model.selected", events)
            self.assertIn("bb.admitted", events)
            self.assertIn("agent.admitted", events)
            self.assertIn("agent.cancel_requested", events)
            self.assertIn("approval.resolved", events)

    def test_autonomous_workflow_runs_reviewed_actions_and_writes_summary(self) -> None:
        from yteam_autonomy import run
        from yteam_state import StateStore

        class ScopeDecision:
            allowed = True
            mode = "fixture"
            reason = "authorized test fixture"

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "run"
            output.mkdir()

            def fake_hunt(target, path, pipeline_id, depth, rate, use_external, scan, scope_file):
                (path / "hypotheses.json").write_text(json.dumps({"hypotheses": [{"id": "h1"}]}), encoding="utf-8")
                (path / "track_plan.json").write_text(json.dumps({"tracks": [{"track": "authorization", "status": "eligible"}]}), encoding="utf-8")
                (path / "recon.json").write_text(json.dumps({"routes": [{"url": target}]}), encoding="utf-8")
                return {"status": "ready_for_analysis", "stages": [{"name": "scope"}], "tool_runs": []}

            store = StateStore(root / "state.db")
            with patch("yteam_scope.validate", return_value=ScopeDecision()), patch("yteam_hunt.run", side_effect=fake_hunt):
                result = run("https://authorized.test", output, "pipeline-1", {}, store, "job-1")
            self.assertEqual(result["status"], "completed")
            self.assertEqual([item["status"] for item in result["results"]], ["completed"] * 5)
            self.assertEqual(result["generation"], 1)
            self.assertTrue((output / "autonomy.json").exists())
            self.assertIn("agent.completed", [item["kind"] for item in store.events("job-1")])

    def test_agent_checkpoint_and_queued_job_cancellation_are_durable(self) -> None:
        from yteam_state import StateStore

        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state.db")
            job = store.create_job("https://authorized.test", kind="autonomous_assessment")
            checkpoint = store.save_agent_checkpoint(job["id"], job["target"], "running", {"rounds": 2, "pending_actions": [{"id": "next"}]})
            self.assertEqual(checkpoint["revision"], 0)
            self.assertEqual(store.agent_run(job["id"])["checkpoint"]["rounds"], 2)
            cancelled = store.request_job_cancel(job["id"])
            self.assertEqual(cancelled["status"], "cancelled")
            self.assertTrue(store.agent_cancel_requested(job["id"]))

            waiting = store.create_job("https://authorized.test", kind="autonomous_assessment")
            store.update_job(waiting["id"], status="waiting_approval", phase="approval")
            approval = store.create_approval("reviewed", waiting["target"], "fixture", {}, job_id=waiting["id"], action_id="risk")
            store.resolve_approval(approval["id"], "approved")
            self.assertEqual(store.job(waiting["id"])["status"], "queued")
            self.assertEqual(store.job(waiting["id"])["phase"], "approval_resolved")

    def test_state_store_migrates_v1_approval_table(self) -> None:
        import sqlite3

        from yteam_state import StateStore

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "state.db"
            connection = sqlite3.connect(path)
            connection.execute(
                "CREATE TABLE approvals (id TEXT PRIMARY KEY, tool_name TEXT NOT NULL, target TEXT NOT NULL, reason TEXT NOT NULL, arguments TEXT NOT NULL DEFAULT '{}', status TEXT NOT NULL DEFAULT 'pending', decided_by TEXT NOT NULL DEFAULT '', created_at REAL NOT NULL, decided_at REAL NOT NULL DEFAULT 0)"
            )
            connection.commit()
            connection.close()
            store = StateStore(path)
            approval = store.create_approval("reviewed", "example.com", "fixture", {}, job_id="job-1", action_id="a1")
            self.assertEqual(approval["job_id"], "job-1")
            self.assertIn("consumed_at", approval)

    def test_memory_requires_verification_before_prompt_context(self) -> None:
        from yteam_memory import LearningMemory

        with tempfile.TemporaryDirectory() as directory:
            memory = LearningMemory(Path(directory) / "learning.jsonl")
            proposal = memory.propose("Fixture API returns 403 for an unknown object", source="test")
            self.assertEqual(memory.context(), "No verified YTEAM lessons are available for this request.")
            verified = memory.verify(str(proposal["id"]), verifier="test")
            self.assertEqual(verified["status"], "verified")
            self.assertIn("Fixture API returns 403", memory.context("API"))

    def test_remote_control_requires_hmac_actor_and_exact_target_allowlist(self) -> None:
        from yteam_control import ControlPlane
        from yteam_runtime import YteamRuntime

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.dict(os.environ, {"YTEAM_CONTROL_SECRET": "control-secret", "YTEAM_TELEGRAM_ALLOWLIST": "chat-1", "YTEAM_REMOTE_TARGET_ALLOWLIST": "https://authorized.test"}, clear=False):
                with patch("yteam_runtime.discover_free_models", return_value=["model-a"]):
                    runtime = YteamRuntime(root)
                plane = ControlPlane(runtime, root)
                body = b'{"actor":"chat-1","text":"/status"}'
                signature = plane.signature(body, "POST", "/webhook/telegram")
                self.assertTrue(plane.verify_signature(body, signature, "POST", "/webhook/telegram"))
                self.assertFalse(plane.verify_signature(body, signature, "POST", "/webhook/discord"))
                self.assertIn('"runtime": "standalone"', plane.execute("telegram", "chat-1", "/status"))
                self.assertIn("not allowlisted", plane.execute("telegram", "other", "/status"))
                self.assertIn("target is not", plane.execute("telegram", "chat-1", "/bb https://other.test"))
                self.assertIn("Queued durable read-only", plane.execute("telegram", "chat-1", "/bb https://authorized.test"))

    def test_state_store_replays_ordered_events_and_redacts_payloads(self) -> None:
        from yteam_state import StateStore

        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state.db")
            first = store.emit("session-1", "tool.started", "Authorization: Bearer live-secret", {"token": "live-token", "ok": True})
            second = store.emit("session-1", "tool.completed", "done", {"value": 2})
            self.assertEqual((first["sequence"], second["sequence"]), (1, 2))
            events = store.events("session-1")
            self.assertEqual([item["sequence"] for item in events], [1, 2])
            self.assertNotIn("live-secret", json.dumps(events))
            self.assertEqual(store.events("session-1", after=1)[0]["kind"], "tool.completed")

    def test_stale_running_job_is_requeued_for_terminal_close_recovery(self) -> None:
        from yteam_state import StateStore

        with tempfile.TemporaryDirectory() as directory:
            store = StateStore(Path(directory) / "state.db")
            job = store.create_job("https://authorized.test")
            claimed = store.claim_job("dead-worker")
            self.assertEqual(claimed["id"], job["id"])
            # The public recovery API uses the heartbeat column; age the row in
            # a controlled fixture to model a process killed with the terminal.
            with store._connection() as connection:
                connection.execute("UPDATE jobs SET heartbeat_at=? WHERE id=?", (time.time() - 100, job["id"]))
            self.assertEqual(store.recover_stale_jobs(45), 1)
            recovered = store.job(job["id"])
            self.assertEqual(recovered["status"], "queued")
            self.assertEqual(recovered["worker_id"], "")

    def test_native_ai_client_streams_sse_without_api_key_for_free(self) -> None:
        from yteam_ai import stream_chat

        class ModelHandler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                request_body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                self.server.seen = {"path": self.path, "body": request_body, "auth": self.headers.get("Authorization")}
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.end_headers()
                self.wfile.write(b'data: {"choices":[{"delta":{"content":"hello"}}]}\n\n')
                self.wfile.write(b'data: [DONE]\n\n')

            def log_message(self, *_args: object) -> None:
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), ModelHandler)
        server.seen = {}
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            config = {"model": "model-a", "base_url": f"http://127.0.0.1:{server.server_port}/v1", "api_key": ""}
            self.assertEqual("".join(stream_chat(config, [{"role": "user", "content": "hi"}])), "hello")
            self.assertEqual(server.seen["path"], "/v1/chat/completions")
            self.assertIsNone(server.seen["auth"])
            self.assertTrue(server.seen["body"]["stream"])
        finally:
            server.shutdown()
            server.server_close()

    def test_native_recon_maps_fixture_and_enforces_attribution(self) -> None:
        from yteam_recon import ReconEngine, route_priority

        score, reasons = route_priority(self.target() + "/api/admin/export", 403, "application/json", "test")
        self.assertGreaterEqual(score, 80)
        self.assertIn("protected response boundary", reasons)
        with tempfile.TemporaryDirectory() as directory:
            engine = ReconEngine(self.target(), Path(directory), depth=1, rate=100, headers={"X-Bug-Bounty": "wrong"})
            self.assertEqual(engine.headers["X-Bug-Bounty"], "pamungkas")
            result = engine.run()
            self.assertGreaterEqual(result["request_count"], 1)
            self.assertTrue((Path(directory) / "recon.json").exists())
            self.assertTrue(any("/api/profile?id=1" in item["url"] for item in result["routes"]))
            self.assertIn("localsolver", result)

    def test_localsolver_allowlist_queue_and_safe_url(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            from local_solver.camoufox_adapter import safe_url, same_origin
            from local_solver.service import LocalSolverService, allowed_target
        finally:
            sys.path.remove(str(ROOT / "src"))
        self.assertTrue(same_origin("https://example.test/a", "https://example.test/b"))
        self.assertNotIn("oauth-live", safe_url("https://example.test/cb?code=oauth-live&next=%2Fhome"))
        self.assertTrue(allowed_target("https://example.test", {"https://example.test"}))
        self.assertFalse(allowed_target("http://127.0.0.1:80", {"https://example.test"}))
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.dict(os.environ, {"LOCALSOLVER_TARGET_ALLOWLIST": "https://example.test"}, clear=False):
                service = LocalSolverService(root, workers=1)
                service.stop.set()
                task = service.submit("https://example.test")
                self.assertEqual(task["status"], "queued")
                self.assertIn("task_id", task)
                with self.assertRaises(PermissionError):
                    service.submit("https://other.test")
                service.close()

    def test_first_party_skill_registry_is_portable_and_risk_aware(self) -> None:
        from yteam_skills import get_skill, registry, source_roots

        # Registry is strictly first-party: it must never traverse vendor trees.
        self.assertTrue(all(str(path).replace("\\", "/").startswith("skills/") or str(path).endswith("skills") for path in source_roots()))
        items = registry()
        names = {str(item["name"]): item for item in items}
        # Reviewed first-party SKILL.md entries load their body on demand.
        self.assertIn("yteam-recon", names)
        loaded = get_skill(items, "yteam-recon")
        self.assertEqual(loaded["access"], "loaded")
        self.assertIn("Recon is complete", loaded["content"])
        # A metadata-only catalog entry with no SKILL.md never loads a body.
        metadata_only = {str(item["name"]): item for item in items if not str(item.get("path", ""))}
        for name, item in metadata_only.items():
            fetched = get_skill(items, name)
            self.assertEqual(fetched["access"], "metadata_only")
            self.assertEqual(fetched["content"], "")
        # A first-party skill whose risk classifies as quarantined is body-blocked.
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "skills"
            risky = root / "reverse-shell-techniques"
            risky.mkdir(parents=True)
            (risky / "SKILL.md").write_text("---\nname: reverse-shell-techniques\ndescription: reverse-shell\n---\n# Payloads\nquarantined\n", encoding="utf-8")
            synthetic = registry() + [
                {
                    "name": "reverse-shell-techniques",
                    "description": "reverse-shell",
                    "path": str(risky / "SKILL.md"),
                    "source": "skills",
                    "categories": ["injection"],
                    "content_sha256": "x",
                    "size_bytes": 0,
                    "line_count": 0,
                    "sections": [],
                    "risk": "quarantined",
                    "load_policy": "metadata_only",
                }
            ]
            blocked = get_skill(synthetic, "reverse-shell-techniques")
            self.assertEqual(blocked["access"], "quarantined")
            self.assertEqual(blocked["content"], "")

    def test_native_hunt_writes_adaptive_context_and_bundle(self) -> None:
        from yteam_hunt import run

        with tempfile.TemporaryDirectory() as directory:
            result = run(self.target(), Path(directory), None, 1, 100, False, False)
            self.assertEqual(result["status"], "ready_for_analysis")
            for name in ("scope.json", "recon.json", "hunt.json", "track_plan.json", "hidden_surface.json", "yteam-skill-registry.json", "yteam-skill-bundle.json", "hunt_context.md"):
                self.assertTrue((Path(directory) / name).exists(), name)
            self.assertFalse((Path(directory) / "cybermes-skill-registry.json").exists())
            self.assertFalse((Path(directory) / "cybermes-skill-bundle.json").exists())

    def test_native_skill_registry_only_reads_first_party_skills(self) -> None:
        from yteam_skills import registry, select_bundle

        items = registry()
        names = {str(item["name"]) for item in items}
        self.assertTrue({"yteam-recon", "yteam-authorization", "yteam-injection", "yteam-reporting", "yteam-runtime"}.issubset(names))
        self.assertTrue(all(not str(item["path"]).startswith("vendor/") for item in items))
        bundle = select_bundle(items, ["graphql", "authorization", "api"], 8)
        self.assertLessEqual(len(bundle), 8)
        self.assertTrue(all("content_sha256" in item for item in bundle))

    def test_native_tools_redact_and_aggregate(self) -> None:
        from yteam_native_tools import aggregate_reports, secret_scan

        self.assertTrue(secret_scan("Authorization: Bearer live-secret"))
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "findings").mkdir()
            (root / "findings" / "high_example.md").write_text("# Example finding\n", encoding="utf-8")
            result = aggregate_reports(root)
            self.assertEqual(result["finding_count"], 1)
            self.assertTrue((root / "SUMMARY.md").exists())
            self.assertTrue((root / "metadata.json").exists())

    def test_scope_origin_and_path_matching_is_consistent(self) -> None:
        from yteam_scope import validate

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scope = root / "scope.yaml"
            scope.write_text(
                "in_scope:\n  - https://allowed.test\n  - https://api.allowed.test/v1\nout_of_scope: []\n",
                encoding="utf-8",
            )
            self.assertTrue(validate("https://allowed.test", explicit=scope).allowed)
            self.assertTrue(validate("https://allowed.test/path/x", explicit=scope).allowed)
            self.assertFalse(validate("https://allowed.test.evil.com", explicit=scope).allowed)
            self.assertFalse(validate("http://allowed.test", explicit=scope).allowed)

    def test_scope_malformed_yaml_blocks(self) -> None:
        from yteam_scope import validate

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            scope = root / "scope.yaml"
            scope.write_text("in_scope: [unclosed\n", encoding="utf-8")
            decision = validate("https://allowed.test", explicit=scope)
            self.assertFalse(decision.allowed)

    def test_recon_same_host_requires_same_origin(self) -> None:
        from yteam_recon import same_host

        self.assertTrue(same_host("https://allowed.test/a", "https://allowed.test"))
        self.assertFalse(same_host("http://allowed.test/a", "https://allowed.test"))
        self.assertFalse(same_host("https://allowed.test.evil.com/a", "https://allowed.test"))
        self.assertFalse(same_host("https://allowed.test:8443/a", "https://allowed.test"))

    def test_installer_is_native_and_dry_run_is_non_destructive(self) -> None:
        installer = (SCRIPTS / "install_yteam.py").read_text(encoding="utf-8")
        self.assertIn('ROOT / "runtime" / ".venv"', installer)
        self.assertIn("yteam_tui.py", installer)
        self.assertIn("yteam_control.py", installer)
        self.assertIn("yteam_worker.py", installer)
        self.assertIn("camoufox", installer.lower())
        self.assertIn("localsolver.py", installer)
        self.assertIn("yteam_mcp.py", installer)
        self.assertIn('"Scripts" if os.name == "nt"', installer)
        self.assertIn('"python.exe" if os.name == "nt"', installer)
        self.assertIn("persist_user_path", installer)
        import install_yteam

        launcher = install_yteam.launcher_text(ROOT)
        if os.name == "nt":
            self.assertIn("runtime\\.venv\\Scripts\\python.exe", launcher)
            self.assertIn("quit.marker", launcher)
            self.assertIn(":loop", launcher)
        else:
            self.assertIn("runtime/.venv/bin/python", launcher)
            self.assertIn("quit.marker", launcher)
            self.assertIn("while :; do", launcher)
        self.assertNotIn("python3 \"", launcher)
        result = subprocess.run([sys.executable, str(SCRIPTS / "install_yteam.py"), "--dry-run"], capture_output=True, text=True, check=False)
        self.assertEqual(result.returncode, 0)
        self.assertIn("standalone YTEAM", result.stdout)

    def test_opencode_style_tui_constructs_both_visual_states(self) -> None:
        from yteam_runtime import YteamRuntime
        from yteam_tui import OpenCodeUI

        with tempfile.TemporaryDirectory() as directory:
            with patch("yteam_runtime.discover_free_models", return_value=["model-test"]):
                runtime = YteamRuntime(Path(directory))
                ui = OpenCodeUI(runtime)
            self.assertIsNotNone(ui.app)
            self.assertFalse(ui.state.snapshot()["workspace"])
            self.assertIn("YTEAM Security Agent", "".join(text for _, text in ui._sidebar_text()))
            self.assertIn("model-test", "".join(text for _, text in ui._model_line()))
            ui.state.add("user", "oi")
            self.assertTrue(ui.state.snapshot()["workspace"])
            self.assertIn("oi", "".join(text for _, text in ui._transcript_text()))

    def test_ci_and_ignore_contract_are_standalone(self) -> None:
        workflow = (ROOT / ".github" / "workflows" / "ci.yml").read_text(encoding="utf-8")
        ignore = (ROOT / ".gitignore").read_text(encoding="utf-8")
        self.assertIn("contents: read", workflow)
        self.assertIn("unittest discover", workflow)
        self.assertNotIn("bootstrap_sources", workflow)
        self.assertNotIn("vendor/cybermes", workflow)
        self.assertIn("vendor/", ignore)
        self.assertIn(".opencode/", ignore)
        self.assertIn("yteam.local.yaml", ignore)


if __name__ == "__main__":
    unittest.main()
