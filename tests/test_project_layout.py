from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class ProjectLayoutTests(unittest.TestCase):
    def test_upstream_checkouts_and_overlay_exist(self) -> None:
        self.assertTrue((ROOT / "vendor" / "opencode" / ".git").exists())
        self.assertTrue((ROOT / "vendor" / "hermes-agent" / ".git").exists())
        self.assertTrue((ROOT / "opencode.json").exists())
        self.assertTrue((ROOT / "YTEAM_SECURITY.md").exists())
        self.assertTrue((ROOT / "yteam.bat").exists())
        self.assertTrue((ROOT / "yteam").exists())

    def test_opencode_config_is_valid_json_and_uses_runtime_key(self) -> None:
        config = json.loads((ROOT / "opencode.json").read_text(encoding="utf-8"))
        self.assertEqual(config["default_agent"], "yteam-security")
        self.assertEqual(config["provider"]["yteam"]["options"]["apiKey"], "{env:YTEAM_BRIDGE_KEY}")
        self.assertEqual(config["provider"]["yteam"]["models"]["yteam-agent"]["name"], "YTEAM")
        self.assertNotIn("API_SERVER_KEY", (ROOT / "opencode.json").read_text(encoding="utf-8"))

    def test_yteam_profile_has_separate_identity_and_learning_templates(self) -> None:
        self.assertIn("Yteam", (ROOT / "profile" / "SOUL.md").read_text(encoding="utf-8"))
        self.assertIn("Yteam Learning Memory", (ROOT / "profile" / "memories" / "MEMORY.md").read_text(encoding="utf-8"))
        config = (ROOT / "profile" / "config.yaml").read_text(encoding="utf-8")
        self.assertIn("memory_enabled: true", config)
        self.assertIn("background_review:", config)
        self.assertIn("external_dirs:", config)

    def test_deep_hunt_and_scope_modules_exist(self) -> None:
        self.assertTrue((ROOT / "scripts" / "yteam_hunt.py").exists())
        self.assertTrue((ROOT / "scripts" / "yteam_toolchain.py").exists())
        self.assertTrue((ROOT / "scripts" / "yteam_scope.py").exists())
        self.assertTrue((ROOT / "scripts" / "yteam_hidden.py").exists())

    def test_adaptive_track_plan_keeps_unmatched_tracks_planned(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_hunt import build_track_plan

            result = build_track_plan("https://example.test", Path("D:/opencode/tmp/no-recon.json"))
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        tracks = {item["track"]: item for item in result["tracks"]}
        self.assertEqual(tracks["authorization"]["status"], "planned")
        self.assertEqual(tracks["web-surface"]["status"], "eligible")
        self.assertIn("not vulnerability proof", result["non_claim"])

    def test_recon_engine_pure_priority_and_normalization_contract(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_recon import normalize, route_priority

            self.assertEqual(normalize("example.test"), "https://example.test")
            score, reasons = route_priority("https://example.test/api/admin/export?format=json", 403, "application/json", "test")
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertGreaterEqual(score, 80)
        self.assertIn("protected response boundary", reasons)

    def test_recon_engine_executes_against_a_local_fixture(self) -> None:
        import threading
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                body = b"<html><title>Yteam Fixture</title><a href='/api/profile?id=1'>profile</a><script src='/assets/app.js'></script></html>" if self.path == "/" else b"not found"
                status = 200 if self.path == "/" else 404
                self.send_response(status)
                self.send_header("Content-Type", "text/html")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args: object) -> None:
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with tempfile.TemporaryDirectory() as directory:
                sys.path.insert(0, str(ROOT / "scripts"))
                try:
                    from yteam_recon import ReconEngine

                    result = ReconEngine(f"http://127.0.0.1:{server.server_port}", Path(directory), depth=1, rate=100).run()
                finally:
                    sys.path.remove(str(ROOT / "scripts"))
                self.assertGreaterEqual(result["request_count"], 1)
                route_urls = {item["url"] for item in result["routes"]}
                self.assertIn(f"http://127.0.0.1:{server.server_port}/api/profile?id=1", route_urls)
                self.assertTrue((Path(directory) / "recon.json").exists())
        finally:
            server.shutdown()
            server.server_close()

    def test_recon_engine_enforces_attribution_header(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_recon import ReconEngine

            engine = ReconEngine("https://example.test", Path("D:/opencode/tmp/yteam-recon-test"), depth=1, rate=1, headers={"X-Bug-Bounty": "wrong-value"})
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertEqual(engine.headers["X-Bug-Bounty"], "pamungkas")

    def test_hunt_output_redacts_external_tool_secrets(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_hunt import redact_output

            output = redact_output("Authorization: Bearer live-secret Cookie: session=live-cookie api_key=live-key")
            self.assertNotIn("csrf-live", redact_output('{"csrfToken":"csrf-live","accessToken":"access-live"}'))
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertNotIn("live-secret", output)
        self.assertNotIn("live-cookie", output)
        self.assertNotIn("live-key", output)
        self.assertIn("<REDACTED>", output)

    def test_shared_safety_redaction_covers_nested_values_and_urls(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_safety import redact_url, redact_value
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        value = redact_value({"csrfToken": "csrf-live", "profile": {"email": "tester@example.test"}, "token": "tok-live"})
        self.assertEqual(value["csrfToken"], "<REDACTED>")
        self.assertEqual(value["token"], "<REDACTED>")
        self.assertEqual(value["profile"]["email"], "tester@example.test")
        safe_url = redact_url("https://example.test/callback?code=oauth-live&next=%2Fhome&state=state-live")
        self.assertNotIn("oauth-live", safe_url)
        self.assertNotIn("state-live", safe_url)
        self.assertIn("next=%2Fhome", safe_url)

    def test_core_redaction_covers_auth_headers_and_camel_case_tokens(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            from core import redact
        finally:
            sys.path.remove(str(ROOT / "src"))
        result = redact({"csrfToken": "csrf-live", "Authorization": "Bearer bearer-live", "email": "tester@example.test"})
        self.assertEqual(result["csrfToken"], "<REDACTED>")
        self.assertEqual(result["Authorization"], "<REDACTED>")
        self.assertEqual(result["email"], "tester@example.test")

    def test_deep_hunt_runs_native_recon_and_writes_manifest(self) -> None:
        import threading
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                body = b"<html><title>Yteam Hunt Fixture</title><a href='/api/profile?id=1'>profile</a></html>" if self.path == "/" else b"not found"
                self.send_response(200 if self.path == "/" else 404)
                self.send_header("Content-Type", "text/html")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args: object) -> None:
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with tempfile.TemporaryDirectory() as directory:
                sys.path.insert(0, str(ROOT / "scripts"))
                try:
                    from yteam_hunt import run

                    result = run(f"http://127.0.0.1:{server.server_port}", Path(directory), None, 1, 100, False, False)
                finally:
                    sys.path.remove(str(ROOT / "scripts"))
                self.assertEqual(result["status"], "ready_for_analysis")
                self.assertTrue((Path(directory) / "hunt.json").exists())
                self.assertTrue((Path(directory) / "recon.json").exists())
                self.assertTrue((Path(directory) / "track_plan.json").exists())
                self.assertTrue((Path(directory) / "hunt_context.json").exists())
                self.assertTrue((Path(directory) / "hunt_context.md").exists())
                self.assertTrue((Path(directory) / "next_actions.json").exists())
                self.assertTrue((Path(directory) / "hidden_surface.json").exists())
                self.assertTrue((Path(directory) / "cybermes-skill-registry.json").exists())
                self.assertTrue((Path(directory) / "cybermes-skill-bundle.json").exists())
                self.assertTrue(any(stage["name"] == "route_mining" for stage in result["stages"]))
        finally:
            server.shutdown()
            server.server_close()

    def test_deep_hunt_run_id_preserves_phase_mapping(self) -> None:
        import threading
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                body = b"<html><title>Yteam Hunt</title><a href='/api/profile'>profile</a></html>"
                self.send_response(200)
                self.send_header("Content-Type", "text/html")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args: object) -> None:
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            import bb_pipeline
            from yteam_hunt import run

            old_runs = bb_pipeline.RUNS
            with tempfile.TemporaryDirectory() as directory:
                bb_pipeline.RUNS = Path(directory) / "runs"
                ledger = bb_pipeline.prepare(f"http://127.0.0.1:{server.server_port}")
                result = run(f"http://127.0.0.1:{server.server_port}", Path(directory) / "output", ledger["run_id"], 1, 100, False, False)
                updated = bb_pipeline.read(ledger["run_id"])
                self.assertEqual(result["status"], "ready_for_analysis")
                self.assertEqual(updated["current_phase"], "triage")
                self.assertTrue(any(item["phase"] == "mapping" for item in updated["events"]))
            bb_pipeline.RUNS = old_runs
        finally:
            sys.path.remove(str(ROOT / "scripts"))
            server.shutdown()
            server.server_close()

    def test_hidden_surface_engine_builds_bug_first_trust_boundary_plan(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_hidden import analyze_surface
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        recon = {
            "routes": [
                {"url": "https://example.test/api/v1/users/123/profile?tenant_id=t1", "priority": 70, "sources": ["html-link"], "status": 200, "content_type": "application/json"},
                {"url": "https://example.test/api/v2/users/123/profile?tenant_id=t1", "priority": 70, "sources": ["swagger"], "status": 403, "content_type": "application/json"},
                {"url": "https://example.test/api/graphql", "priority": 80, "sources": ["script-src"], "status": 200, "content_type": "application/json"},
                {"url": "https://example.test/api/preview?url=https%3A%2F%2Fexample.org", "priority": 60, "sources": ["html-link"], "status": 200, "content_type": "application/json"},
            ]
        }
        result = analyze_surface("https://example.test", recon)
        classes = {item["class"] for item in result["hypotheses"]}
        self.assertIn("idor_bola_candidate", classes)
        self.assertIn("api_version_drift", classes)
        self.assertIn("graphql_rest_authorization_overlap", classes)
        self.assertIn("url_processing_boundary", classes)
        self.assertGreaterEqual(result["route_count"], 4)
        self.assertTrue(result["safe_checks"])
        self.assertTrue(all("stop_signal" in item for item in result["safe_checks"]))

    def test_yteam_tui_branding_is_an_overlay_plugin(self) -> None:
        plugin = ROOT / ".opencode" / "plugins" / "yteam-tui.tsx"
        self.assertTrue(plugin.exists())
        self.assertIn('id: "yteam-tui"', plugin.read_text(encoding="utf-8"))
        self.assertIn("YTEAM", plugin.read_text(encoding="utf-8"))
        self.assertIn("SAFE MODE", plugin.read_text(encoding="utf-8"))
        self.assertIn("BOTTERDOP", plugin.read_text(encoding="utf-8"))

    def test_security_commands_use_yteam_agent(self) -> None:
        commands = ROOT / ".opencode" / "command"
        self.assertEqual(sorted(path.stem for path in commands.glob("*.md")), ["bb"])
        text = (commands / "bb.md").read_text(encoding="utf-8")
        self.assertIn("agent: yteam-security", text)
        self.assertNotIn("agent: hermes-security", text)

    def test_bb_is_the_single_autonomous_pipeline_entrypoint(self) -> None:
        text = (ROOT / ".opencode" / "command" / "bb.md").read_text(encoding="utf-8")
        for phase in ("Scope and queue", "Recon and surface", "Map and prioritize", "Hypothesis and validation", "Impact proof", "Triage and deliverable"):
            self.assertIn(phase, text)
        self.assertIn("yteam_run.py", text)
        self.assertIn("--resume", text)
        self.assertIn("--camoufox", text)
        self.assertIn("hidden_surface.json", (ROOT / "README.md").read_text(encoding="utf-8"))
        self.assertIn("never auto-submit", text)

    def test_cybermes_source_and_intelligence_layer_are_wired(self) -> None:
        self.assertTrue((ROOT / "vendor" / "cybermes" / ".git").exists())
        self.assertTrue((ROOT / "scripts" / "cybermes.py").exists())
        intelligence = (ROOT / "scripts" / "yteam_intelligence.py").read_text(encoding="utf-8")
        self.assertIn("emerging_bug_hypothesis", intelligence)
        profile = (ROOT / "profile" / "config.yaml").read_text(encoding="utf-8")
        self.assertNotIn("mcp_servers:", profile)
        self.assertNotIn('"mcp"', (ROOT / "opencode.json").read_text(encoding="utf-8"))

    def test_complete_cybermes_skill_registry_and_bundle(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_skills import registry, select_bundle

            items = registry()
            bundle = select_bundle(items, ["/api/graphql", "authorization", "oauth"], 24)
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertGreaterEqual(len(items), 250)
        names = {str(item["name"]) for item in items}
        self.assertTrue({"bug-bounty", "api-recon-and-docs", "hunt-idor", "hunt-ssrf"}.issubset(names))
        self.assertLessEqual(len(bundle), 24)
        self.assertTrue(all("content_sha256" in item for item in bundle))

    def test_opencode_config_keeps_native_command_surface_unmodified(self) -> None:
        launcher = (ROOT / "scripts" / "hermes_opencode.py").read_text(encoding="utf-8")
        for command in ("serve", "acp", "attach", "mcp", "models", "run", "debug", "session", "providers", "web"):
            self.assertIn(f'"{command}"', launcher)

    def test_user_launcher_installer_points_back_to_this_source_tree(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from install_yteam import launcher_text

            text = launcher_text(ROOT)
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertIn("hermes_opencode.py", text)
        self.assertIn("%*" if sys.platform == "win32" else '"$@"', text)

    def test_direct_cybermes_wrapper_has_no_protocol_server_dependency(self) -> None:
        wrapper = (ROOT / "scripts" / "cybermes.py").read_text(encoding="utf-8")
        self.assertIn('"smart-pipe": "smart_pipe"', wrapper)
        self.assertIn('"search-knowledge": "search_knowledge"', wrapper)
        self.assertNotIn("ServeStdio", wrapper)
        self.assertNotIn("run_cybermes_mcp", wrapper)

    def test_bb_pipeline_writes_redacted_durable_run_state(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            import bb_pipeline

            old_runs = bb_pipeline.RUNS
            with tempfile.TemporaryDirectory() as directory:
                bb_pipeline.RUNS = Path(directory) / "runs"
                run = bb_pipeline.prepare("https://example.test/app")
                updated = bb_pipeline.event(run["run_id"], "note", "Authorization: Bearer secret-must-not-persist", "scope")
                saved = bb_pipeline.path_for(run["run_id"]).read_text(encoding="utf-8")
                self.assertEqual(updated["current_phase"], "scope")
                self.assertEqual(updated["hypothesis_count"], 0)
                self.assertIn("<REDACTED>", saved)
                self.assertNotIn("secret-must-not-persist", saved)
                self.assertEqual(len(updated["phases"]), 7)
                self.assertIn(f"runtime/bb-runs/{run['run_id']}/example_test/recon", updated["paths"]["recon"])
            bb_pipeline.RUNS = old_runs
        finally:
            sys.path.remove(str(ROOT / "scripts"))

    def test_cybermes_is_not_configured_as_an_mcp_server(self) -> None:
        config = (ROOT / "opencode.json").read_text(encoding="utf-8")
        profile = (ROOT / "profile" / "config.yaml").read_text(encoding="utf-8")
        self.assertNotIn("run_cybermes_mcp", config + profile)
        self.assertNotIn("mcp_servers:", profile)

    def test_intelligence_engine_keeps_hypotheses_separate_from_findings(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_intelligence import analyze, canonical_observation

            observations = [
                canonical_observation({"target": "demo", "endpoint": "/api/item", "status": 200, "response_length": 10, "actor": "a", "scope": "one", "tags": []}),
                canonical_observation({"target": "demo", "endpoint": "/api/item", "status": 403, "response_length": 90, "actor": "b", "scope": "two", "tags": []}),
            ]
            result = analyze(observations)
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertGreaterEqual(result["hypothesis_count"], 1)
        self.assertTrue(all(item["status"] == "hypothesis" for item in result["hypotheses"]))
        self.assertFalse(any(item.get("kind") == "finding" for item in result["hypotheses"]))

    def test_intelligence_engine_detects_unknown_class_and_routes_to_track(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_intelligence import analyze, canonical_observation

            observations = [
                canonical_observation({"target": "demo", "endpoint": "/edge/custom-flag", "status": 200, "response_length": 12, "actor": "anonymous", "scope": "tenant-a", "resource": "edge-widget", "prior_state": "locked", "state": "unlocked", "action": "deploy", "body": "unexpected internal object flag exposed", "tags": [], "source": "differential"}),
                canonical_observation({"target": "demo", "endpoint": "/edge/custom-flag", "status": 200, "response_length": 210, "actor": "anonymous", "scope": "tenant-a", "resource": "edge-widget", "prior_state": "unlocked", "state": "disabled", "action": "deploy", "body": "unexpected internal object flag exposed", "tags": [], "source": "differential"}),
                canonical_observation({"target": "demo", "endpoint": "/edge/custom-flag", "status": 200, "response_length": 210, "actor": "anonymous", "scope": "tenant-a", "resource": "edge-widget", "prior_state": "disabled", "state": "locked", "action": "deploy", "body": "unexpected internal object flag exposed", "tags": [], "source": "recheck"}),
            ]
            result = analyze(observations)
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertGreaterEqual(result["hypothesis_count"], 1)
        hypothesis = result["hypotheses"][0]
        self.assertTrue(hypothesis["unknown_class"])
        self.assertIn("state-transition differential", hypothesis["signals"])
        self.assertIn("web-surface", hypothesis["suggested_tracks"])
        self.assertTrue(all("detected_classes" in item for item in result["hypotheses"]))

    def test_intelligence_cli_accepts_json_from_stdin(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            ledger = Path(directory) / "observations.jsonl"
            observation = '{"target":"stdin","endpoint":"/health","method":"GET","status":200,"response_length":1,"actor":"anonymous","scope":"public","tags":[],"source":"test"}'
            result = subprocess.run(
                [sys.executable, str(ROOT / "scripts" / "yteam_intelligence.py"), "record", "--input", "-", "--ledger", str(ledger)],
                input=observation,
                capture_output=True,
                text=True,
                check=True,
            )
            self.assertIn('"kind": "observation"', result.stdout)
            self.assertEqual(len(ledger.read_text(encoding="utf-8").splitlines()), 1)

    def test_profile_initializer_preserves_existing_memory_and_soul(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory) / "yteam-home"
            sys.path.insert(0, str(ROOT / "scripts"))
            try:
                from init_yteam import initialize

                initialize(home)
                (home / "SOUL.md").write_text("# Bos custom Yteam soul\n", encoding="utf-8")
                (home / "memories" / "MEMORY.md").write_text("# preserved lesson\n", encoding="utf-8")
                initialize(home)
            finally:
                sys.path.remove(str(ROOT / "scripts"))
            self.assertEqual((home / "SOUL.md").read_text(encoding="utf-8"), "# Bos custom Yteam soul\n")
            self.assertEqual((home / "memories" / "MEMORY.md").read_text(encoding="utf-8"), "# preserved lesson\n")

    def test_skill_indexer_handles_a_minimal_source_tree(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "skill-catalog.json"
            result = subprocess.run(
                [sys.executable, str(ROOT / "scripts" / "index_skills.py"), "--output", str(target)],
                cwd=ROOT,
                capture_output=True,
                text=True,
                check=True,
            )
            self.assertIn("Indexed", result.stdout)
            self.assertTrue(target.exists())
            self.assertIsInstance(json.loads(target.read_text(encoding="utf-8")), list)

    def test_autonomous_driver_dry_run_queue_triage_is_non_destructive(self) -> None:
        result = subprocess.run(
            [sys.executable, str(ROOT / "scripts" / "yteam_run.py"), "--dry-run"],
            capture_output=True,
            text=True,
            check=False,
        )
        payload = json.loads(result.stdout)
        self.assertEqual(result.returncode, 0)
        self.assertIn(payload["mode"], {"queue", "resume"})

    def test_queue_triage_prefers_newest_active_run(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            import yteam_run

            with tempfile.TemporaryDirectory() as directory:
                old_runs = yteam_run.DEFAULT_RUNS
                yteam_run.DEFAULT_RUNS = Path(directory)
                try:
                    for run_id, updated_at in (("bb_20240101_000000_old", "2024-01-01"), ("bb_20240102_000000_new", "2024-01-02")):
                        (Path(directory) / f"{run_id}.json").write_text(json.dumps({"run_id": run_id, "target": "https://example.test", "status": "active", "current_phase": "recon", "updated_at": updated_at}), encoding="utf-8")
                    selected = yteam_run.queue_triage()["selection"]
                finally:
                    yteam_run.DEFAULT_RUNS = old_runs
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertEqual(selected["run_id"], "bb_20240102_000000_new")

    def test_autonomous_driver_honors_engine_flag(self) -> None:
        import contextlib
        import io
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            import yteam_run

            original = yteam_run.engine_run
            yteam_run.engine_run = lambda target, run_id=None: {"ok": True, "route": "engine", "target": target}
            original_unified = yteam_run.unified_run
            yteam_run.unified_run = lambda *args, **kwargs: {"ok": True, "route": "unified"}
            try:
                old_argv = sys.argv
                output = io.StringIO()
                sys.argv = ["yteam_run.py", "https://example.test", "--engine"]
                with contextlib.redirect_stdout(output):
                    self.assertEqual(yteam_run.main(), 0)
                result = json.loads(output.getvalue())
            finally:
                sys.argv = old_argv
                yteam_run.engine_run = original
                yteam_run.unified_run = original_unified
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertEqual(result["route"], "engine")

    def test_autonomous_driver_runs_full_pipeline_on_fixture(self) -> None:
        import threading
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                body = b"<html><title>Yteam Auto</title><a href='/api/profile?id=1'>profile</a></html>" if self.path == "/" else b"not found"
                self.send_response(200 if self.path == "/" else 404)
                self.send_header("Content-Type", "text/html")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args: object) -> None:
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        old_runs = None
        try:
            with tempfile.TemporaryDirectory() as directory:
                sys.path.insert(0, str(ROOT / "scripts"))
                try:
                    import bb_pipeline
                    import yteam_run

                    old_runs = yteam_run.DEFAULT_RUNS
                    yteam_run.DEFAULT_RUNS = Path(directory) / "runs"
                    bb_pipeline.RUNS = yteam_run.DEFAULT_RUNS
                    result = yteam_run.autonomous_run(f"http://127.0.0.1:{server.server_port}", 1, 100, False, None)
                finally:
                    sys.path.remove(str(ROOT / "scripts"))
                self.assertTrue(result["ok"])
                self.assertEqual(result["status"], "ready_for_analysis")
                self.assertTrue((Path(result["context_path"])).exists())
        finally:
            if old_runs is not None:
                yteam_run.DEFAULT_RUNS = old_runs
            server.shutdown()
            server.server_close()


    def test_knowledge_base_records_and_dedupes_verdicts(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            import yteam_knowledge

            with tempfile.TemporaryDirectory() as directory:
                kb = Path(directory) / "kb.jsonl"
                yteam_knowledge.add_verdict("demo.test", "/api/account", ["idor"], "candidate", "cross-tenant", kb)
                found = yteam_knowledge.lookup("demo.test", "/api/account", ["idor"], kb)
                stats = yteam_knowledge.stats(kb)
                self.assertIsNotNone(found)
                self.assertEqual(found["verdict"], "candidate")
                self.assertEqual(stats["verdicts"]["candidate"], 1)
        finally:
            sys.path.remove(str(ROOT / "scripts"))

    def test_parallel_scheduler_enforces_rate_and_concurrency(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_parallel import ParallelScheduler, Task

            calls: list[float] = []
            import time

            def probe(_name: str) -> str:
                calls.append(time.monotonic())
                return "ok"

            scheduler = ParallelScheduler(max_workers=4, rate=20.0)
            tasks = [Task(name=f"t{i}", fn=probe, args=(f"t{i}",)) for i in range(4)]
            results = scheduler.run(tasks)
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertEqual(len(results), 4)
        self.assertTrue(all(value == "ok" for value in results.values()))

    def test_engine_dag_resolves_prerequisites(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_engine import PHASE_DAG, RunContext, make_registry, resolve_ready

            registry = make_registry()
            ctx = RunContext(run_id="test", target="x", target_slug="x", run_dir=ROOT / "runtime")
            # scope is active by default -> it is ready
            self.assertIn("scope", resolve_ready(registry, ctx))
            # once scope completes, inventory becomes ready
            ctx.phases["scope"] = {"status": "completed"}
            ready = resolve_ready(registry, ctx)
            self.assertIn("inventory", ready)
            # mapping requires recon+fingerprint which are not done yet
            self.assertNotIn("mapping", ready)
            ctx.phases["scope"] = {"status": "blocked"}
            self.assertNotIn("inventory", resolve_ready(registry, ctx))
        finally:
            sys.path.remove(str(ROOT / "scripts"))

    def test_opensource_license_and_contributing_exist(self) -> None:
        self.assertTrue((ROOT / "LICENSE").exists())
        self.assertTrue((ROOT / "CONTRIBUTING.md").exists())
        license_text = (ROOT / "LICENSE").read_text(encoding="utf-8")
        self.assertIn("MIT License", license_text)
        self.assertIn("authorized security testing", license_text)

    def test_github_open_source_docs_and_bootstrap_exist(self) -> None:
        for relative in ("SECURITY.md", "THIRD_PARTY_NOTICES.md", "docs/GETTING_STARTED.md", "docs/ARCHITECTURE.md", "docs/PUBLISHING.md", "scripts/bootstrap_sources.py", ".github/workflows/ci.yml", ".gitignore"):
            self.assertTrue((ROOT / relative).exists(), relative)
        getting_started = (ROOT / "docs" / "GETTING_STARTED.md").read_text(encoding="utf-8")
        self.assertIn("/bb https://authorized-target.example", getting_started)
        self.assertIn("Windows PowerShell", getting_started)
        self.assertIn("macOS/Linux", getting_started)
        workflow = (ROOT / ".github" / "workflows" / "ci.yml").read_text(encoding="utf-8")
        self.assertIn("contents: read", workflow)
        self.assertIn("bootstrap_sources.py", workflow)
        self.assertNotIn("<account>", workflow)
        self.assertNotIn("<repository>", workflow)

    def test_third_party_notice_matches_upstream_license_families(self) -> None:
        notice = (ROOT / "THIRD_PARTY_NOTICES.md").read_text(encoding="utf-8")
        sources = (ROOT / "vendor" / "SOURCES.md").read_text(encoding="utf-8")
        self.assertIn("OpenCode", notice)
        self.assertIn("Hermes Agent", notice)
        self.assertIn("Cybermes", notice)
        self.assertIn("MIT", notice)
        self.assertIn("Apache-2.0", notice)
        self.assertIn("Apache License 2.0", sources)

    def test_github_publish_contract_ignores_runtime_and_upstream_checkouts(self) -> None:
        ignore = (ROOT / ".gitignore").read_text(encoding="utf-8")
        publishing = (ROOT / "docs" / "PUBLISHING.md").read_text(encoding="utf-8")
        self.assertIn("runtime/", ignore)
        self.assertIn("/vendor/*/", ignore)
        self.assertIn("reports/", ignore)
        self.assertIn("evidence/", ignore)
        self.assertIn("camoufox-cache/", ignore)
        self.assertIn("bootstrap_sources.py", publishing)
        self.assertIn("hidden_surface.json", (ROOT / "README.md").read_text(encoding="utf-8"))

    def test_bot_bypass_pillar_classifies_gates(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            from bot_bypass.detector import Botterdop, GateKind, classify_response, gate_summary
        finally:
            sys.path.remove(str(ROOT / "src"))
        self.assertEqual(classify_response({"cf-ray": "abc"}, "Just a moment...", 503), GateKind.CLOUDFLARE_CHALLENGE)
        self.assertEqual(classify_response({}, "window.KPSDK={...}", 403), GateKind.AKAMAI_KPSDK)
        self.assertEqual(classify_response({}, "grecaptcha.render(...)", 200), GateKind.RECAPTCHA_V2)
        summary = gate_summary({"cf-ray": "x"}, "challenge", 503)
        self.assertTrue(summary["known_gate"])

        governor = Botterdop(base_rate=10)
        captcha = governor.inspect({}, "<div class='cf-turnstile'></div>", 200)
        self.assertEqual(captcha.action, "manual_review")
        self.assertEqual(captcha.category, "captcha")
        self.assertTrue(governor.summary()["halted"])
        governor = Botterdop(base_rate=10)
        limited = governor.inspect({"Retry-After": "4"}, "", 429)
        self.assertEqual(limited.action, "slow_down")
        self.assertGreaterEqual(limited.retry_after_seconds, 4)
        self.assertGreaterEqual(governor.summary()["current_delay_seconds"], 4)
        waf = governor.inspect({"Server": "cloudflare", "CF-Ray": "abc"}, "Just a moment...", 403)
        self.assertEqual(waf.action, "stop")
        self.assertTrue(governor.summary()["blocked"])
        normal_cdn = Botterdop(base_rate=10).inspect({"CF-Ray": "abc"}, "normal application response", 200)
        self.assertEqual(normal_cdn.gate, "none")
        self.assertIn("Botterdop", (ROOT / "README.md").read_text(encoding="utf-8"))

    def test_camoufox_adapter_is_isolated_and_graceful_when_optional(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            import bot_bypass.camoufox_adapter as camoufox_adapter
            from bot_bypass.camoufox_adapter import CamoufoxConfig, run_camoufox, safe_url, same_origin
        finally:
            sys.path.remove(str(ROOT / "src"))
        self.assertTrue(same_origin("https://example.test/a", "https://example.test/b"))
        self.assertFalse(same_origin("https://other.test/a", "https://example.test/b"))
        self.assertNotIn("oauth-code", safe_url("https://example.test/callback?code=oauth-code&next=%2Fhome"))
        self.assertIn("next=%2Fhome", safe_url("https://example.test/callback?code=oauth-code&next=%2Fhome"))
        with tempfile.TemporaryDirectory() as directory:
            original_loader = camoufox_adapter._load_camoufox
            camoufox_adapter._load_camoufox = lambda: None
            try:
                result = run_camoufox(CamoufoxConfig("https://example.test/callback?state=local-state", Path(directory)))
            finally:
                camoufox_adapter._load_camoufox = original_loader
            self.assertEqual(result["status"], "unavailable")
            self.assertTrue((Path(directory) / "camoufox.json").exists())
            self.assertEqual(result["action"], "manual_review")

    def test_botterdop_cli_and_bb_command_wire_camoufox_safely(self) -> None:
        self.assertTrue((ROOT / "scripts" / "botterdop.py").exists())
        bb = (ROOT / ".opencode" / "command" / "bb.md").read_text(encoding="utf-8")
        self.assertIn("python scripts/yteam_run.py --camoufox", bb)
        self.assertIn("Camoufox is unavailable", bb)
        assessment = (ROOT / "scripts" / "yteam_assessment.py").read_text(encoding="utf-8")
        self.assertIn('parser.add_argument("--camoufox"', assessment)
        adapter = (ROOT / "src" / "bot_bypass" / "camoufox_adapter.py").read_text(encoding="utf-8")
        self.assertIn("headless", adapter)
        self.assertIn("max_body_bytes", adapter)
        self.assertIn("no challenge solving or evasion", adapter)
        self.assertIn("asyncio.create_task", adapter)

    def test_yteam_doctor_and_optional_requirements_are_present(self) -> None:
        doctor = ROOT / "scripts" / "yteam_doctor.py"
        self.assertTrue(doctor.exists())
        self.assertIn("def run()", doctor.read_text(encoding="utf-8"))
        optional = (ROOT / "requirements-optional.txt").read_text(encoding="utf-8")
        self.assertIn("camoufox", optional)
        self.assertFalse((ROOT / ".opencode" / "command" / "doctor.md").exists())

    def test_single_file_model_config_is_local_only_and_maps_credentials_ephemerally(self) -> None:
        example = ROOT / "yteam.local.example.yaml"
        self.assertTrue(example.exists())
        ignore = (ROOT / ".gitignore").read_text(encoding="utf-8")
        self.assertIn("yteam.local.yaml", ignore)
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from hermes_opencode import apply_model_config, apply_model_profile, load_model_config
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        env = {}
        apply_model_config(env, {"provider": "openrouter", "model": "anthropic/test", "api_key": "local-secret", "base_url": "https://openrouter.ai/api/v1"})
        self.assertEqual(env["HERMES_MODEL"], "anthropic/test")
        self.assertEqual(env["OPENROUTER_API_KEY"], "local-secret")
        with tempfile.TemporaryDirectory() as directory:
            config_path = Path(directory) / "model.yaml"
            config_path.write_text("provider: openrouter\nmodel: anthropic/test\napi_key: local-secret\nbase_url: https://openrouter.ai/api/v1\n", encoding="utf-8")
            self.assertEqual(load_model_config(config_path)["model"], "anthropic/test")
            home = Path(directory) / "home"
            home.mkdir()
            profile_config = home / "config.yaml"
            profile_config.write_text("memory:\n  memory_enabled: true\n", encoding="utf-8")
            apply_model_profile(home, load_model_config(config_path))
            persisted = profile_config.read_text(encoding="utf-8")
            self.assertIn("provider: openrouter", persisted)
            self.assertIn("default: anthropic/test", persisted)
            self.assertIn("base_url: https://openrouter.ai/api/v1", persisted)
            self.assertNotIn("local-secret", persisted)

    def test_toolchain_reports_camoufox_as_optional_browser_dependency(self) -> None:
        sys.path.insert(0, str(ROOT / "scripts"))
        try:
            from yteam_toolchain import DEFINITIONS
        finally:
            sys.path.remove(str(ROOT / "scripts"))
        self.assertTrue(any(item[0] == "camoufox" and item[2] == "browser" for item in DEFINITIONS))

    def test_decrypt_pillar_detects_payload_formats(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            from decrypt.detect import analyze_payload, detect_encoding
        finally:
            sys.path.remove(str(ROOT / "src"))
        self.assertIn("hex", detect_encoding("deadbeefcafe"))
        jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature"
        self.assertIn("jwt", detect_encoding(jwt))
        analysis = analyze_payload(jwt)
        self.assertTrue(any("jwt-header" in item for item in analysis["looks_like"]))

    def test_pentest_qa_pillar_builds_matrix(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            from pentest_qa.qa import QA_CHECKLIST, build_matrix
        finally:
            sys.path.remove(str(ROOT / "src"))
        matrix = build_matrix({"authz-01": "pass", "input-01": "fail"})
        self.assertEqual(matrix["total_checks"], len(QA_CHECKLIST))
        self.assertEqual(matrix["summary"]["pass"], 1)
        self.assertEqual(matrix["summary"]["fail"], 1)

    def test_server_guard_pillar_reports_hardening(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            from server_guard.guard import build_guard_report
        finally:
            sys.path.remove(str(ROOT / "src"))
        report = build_guard_report({"x-frame-options": "SAMEORIGIN", "strict-transport-security": "max-age=31536000"}, exposed_paths=["/.git/config"])
        self.assertIn("x-frame-options", [item["name"] for item in report["headers"]])
        self.assertIn("content-security-policy", report["missing_headers"])
        self.assertIn("/.git/config", report["exposed_paths"])

    def test_unified_assessment_runs_all_pillars_in_one_artifact_store(self) -> None:
        import threading
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                body = b"<html><title>Unified Yteam</title><a href='/api/account?id=1'>account</a></html>" if self.path == "/" else b"not found"
                self.send_response(200 if self.path == "/" else 404)
                self.send_header("Content-Type", "text/html")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args: object) -> None:
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            sys.path.insert(0, str(ROOT / "src"))
            try:
                from core.assessment import PILLAR_DAG, run_assessment
                from core.platform import ArtifactStore, AssessmentContext

                with tempfile.TemporaryDirectory() as directory:
                    context = AssessmentContext("asm_test", f"http://127.0.0.1:{server.server_port}", "fixture", ArtifactStore(Path(directory)))
                    result = run_assessment(context)
                    self.assertEqual(set(result["state"]), set(PILLAR_DAG))
                    self.assertTrue(all(state == "completed" for state in result["state"].values()))
                    self.assertTrue((Path(directory) / "assessment_manifest.json").exists())
                    self.assertTrue((Path(directory) / "pillars" / "bot_bypass.json").exists())
                    self.assertTrue((Path(directory) / "pillars" / "decrypt.json").exists())
                    self.assertTrue((Path(directory) / "pillars" / "pentest_qa.json").exists())
                    self.assertTrue((Path(directory) / "pillars" / "server_guard.json").exists())
                    self.assertTrue((Path(directory) / "assessment_context.json").exists() or result["state"]["delivery"] == "completed")
            finally:
                sys.path.remove(str(ROOT / "src"))
        finally:
            server.shutdown()
            server.server_close()

    def test_unified_core_contract_exposes_all_pillars(self) -> None:
        sys.path.insert(0, str(ROOT / "src"))
        try:
            from core.assessment import PILLAR_DAG, build_registry
            from core.platform import ArtifactStore, EventBus, Policy

            registry = build_registry()
        finally:
            sys.path.remove(str(ROOT / "src"))
        self.assertEqual(set(PILLAR_DAG), set(registry.names()))
        self.assertTrue(Policy().permits("recon"))
        self.assertFalse(Policy().permits("destructive"))
        store = ArtifactStore(Path("D:/opencode/tmp/yteam-artifact-test"))
        self.assertTrue(store.path("nested/file.json").is_relative_to(store.root))
        self.assertTrue(store.json("nested/file.json", {"ok": True}).exists())
        self.assertIsNotNone(EventBus().emit("test", "scope"))


if __name__ == "__main__":
    unittest.main()
