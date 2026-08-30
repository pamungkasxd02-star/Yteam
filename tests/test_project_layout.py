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
            "yteam_session.py", "yteam_native_tools.py", "yteam_hunt.py",
        )
        for name in required:
            self.assertTrue((SCRIPTS / name).exists(), name)
        self.assertFalse((ROOT / "opencode.json").exists())
        self.assertFalse((SCRIPTS / "hermes_opencode.py").exists())
        self.assertFalse((SCRIPTS / "bootstrap_sources.py").exists())

    def test_profile_and_docs_describe_standalone_runtime(self) -> None:
        config = (ROOT / "profile" / "config.yaml").read_text(encoding="utf-8")
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        self.assertIn("mode: standalone", config)
        self.assertIn("/bb <authorized-http-target>", readme)
        self.assertIn("no required vendored agent", readme)
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
            self.assertIn("Goodbye", runtime.command("/quit"))
            self.assertTrue(runtime.quit_requested)
            events = (root / "runtime" / "events.jsonl").read_text(encoding="utf-8")
            self.assertIn("model.selected", events)
            self.assertIn("bb.admitted", events)

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

    def test_installer_is_native_and_dry_run_is_non_destructive(self) -> None:
        installer = (SCRIPTS / "install_yteam.py").read_text(encoding="utf-8")
        self.assertIn('ROOT / "runtime" / ".venv"', installer)
        self.assertIn("yteam_tui.py", installer)
        self.assertIn("yteam_control.py", installer)
        self.assertIn("yteam_worker.py", installer)
        self.assertIn("yteam_control.py", installer)
        self.assertNotIn("bootstrap_sources", installer)
        self.assertNotIn("full-sources", installer)
        self.assertNotIn("with-opencode", installer)
        result = subprocess.run([sys.executable, str(SCRIPTS / "install_yteam.py"), "--dry-run"], capture_output=True, text=True, check=False)
        self.assertEqual(result.returncode, 0)
        self.assertIn("standalone YTEAM", result.stdout)

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
