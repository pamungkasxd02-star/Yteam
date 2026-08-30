#!/usr/bin/env python3
"""Secure local control plane for Telegram and signed Discord/WhatsApp bridges."""

from __future__ import annotations

import argparse
import hashlib
import hmac
import json
import os
import threading
import time
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Callable

from yteam_safety import redact_text


def _ids(name: str) -> set[str]:
    return {item.strip() for item in os.environ.get(name, "").split(",") if item.strip()}


class ControlPlane:
    """Command gate shared by local HTTP webhooks and Telegram polling."""

    def __init__(self, runtime, root: Path) -> None:
        self.runtime = runtime
        self.root = root
        self.secret = os.environ.get("YTEAM_CONTROL_SECRET", "")
        self.telegram_token = os.environ.get("YTEAM_TELEGRAM_BOT_TOKEN", "")
        self.allowlists = {
            "telegram": _ids("YTEAM_TELEGRAM_ALLOWLIST"),
            "discord": _ids("YTEAM_DISCORD_ALLOWLIST"),
            "whatsapp": _ids("YTEAM_WHATSAPP_ALLOWLIST"),
        }
        self.target_allowlist = _ids("YTEAM_REMOTE_TARGET_ALLOWLIST")
        self.executor: Callable[[str], None] | None = None
        self.events = runtime.events

    def allowed(self, provider: str, actor: str) -> bool:
        allowed = self.allowlists.get(provider, set())
        return bool(allowed and actor in allowed)

    def verify_signature(self, body: bytes, signature: str, method: str = "POST", path: str = "") -> bool:
        if not self.secret or not signature:
            return False
        message = method.upper().encode() + b"\n" + path.encode() + b"\n" + body
        expected = "sha256=" + hmac.new(self.secret.encode(), message, hashlib.sha256).hexdigest()
        return hmac.compare_digest(expected, signature)

    def signature(self, body: bytes = b"", method: str = "POST", path: str = "/webhook") -> str:
        message = method.upper().encode() + b"\n" + path.encode() + b"\n" + body
        return "sha256=" + hmac.new(self.secret.encode(), message, hashlib.sha256).hexdigest()

    def execute(self, provider: str, actor: str, text: str) -> str:
        text = redact_text(text).strip()
        if not self.allowed(provider, actor):
            self.events.emit("control.denied", f"{provider}:{actor}")
            return "Remote control denied: actor is not allowlisted."
        if not text.startswith("/"):
            return "Commands only. Use /status, /models, /model, /history, /memory, /learn, or /bb."
        if text.startswith("/bb "):
            target = text[4:].strip()
            if self.target_allowlist and target not in self.target_allowlist:
                return "Remote /bb denied: target is not in YTEAM_REMOTE_TARGET_ALLOWLIST."
            if not self.target_allowlist:
                return "Remote /bb disabled until YTEAM_REMOTE_TARGET_ALLOWLIST is configured."
        if text in {"/quit", "/exit", "/q"}:
            return "Remote shutdown is disabled; stop the local service explicitly."
        result = self.runtime.command(text)
        if result is None:
            return "Unknown or unavailable remote command. Use /help."
        self.events.emit("control.executed", f"{provider}:{actor}:{text.split()[0]}")
        if self.runtime.pending_bb_target and self.executor:
            target = self.runtime.pending_bb_target
            self.runtime.pending_bb_target = None
            threading.Thread(target=self.executor, args=(target,), daemon=True).start()
            result += "\nAssessment started in the background; poll /status for local state."
        return result[:3800]

    def start_telegram(self) -> threading.Thread | None:
        if not self.telegram_token or not self.allowlists.get("telegram"):
            return None
        thread = threading.Thread(target=self._telegram_loop, name="yteam-telegram", daemon=True)
        thread.start()
        return thread

    def _telegram_loop(self) -> None:
        offset = 0
        base = f"https://api.telegram.org/bot{self.telegram_token}"
        while True:
            try:
                query = urllib.parse.urlencode({"timeout": 20, "offset": offset})
                with urllib.request.urlopen(f"{base}/getUpdates?{query}", timeout=30) as response:
                    payload = json.loads(response.read().decode("utf-8"))
                for update in payload.get("result", []):
                    offset = max(offset, int(update.get("update_id", 0)) + 1)
                    message = update.get("message") or {}
                    chat = message.get("chat") or {}
                    actor = str(chat.get("id", ""))
                    result = self.execute("telegram", actor, str(message.get("text", "")))
                    data = json.dumps({"chat_id": actor, "text": result}).encode("utf-8")
                    request = urllib.request.Request(f"{base}/sendMessage", data=data, method="POST", headers={"Content-Type": "application/json"})
                    urllib.request.urlopen(request, timeout=15).read()
            except Exception as error:  # noqa: BLE001
                self.events.emit("control.telegram_error", type(error).__name__)
                time.sleep(5)


class Handler(BaseHTTPRequestHandler):
    plane: ControlPlane

    def do_GET(self) -> None:
        if self.path == "/health":
            self._json(200, {"ok": True, "service": "yteam-control"})
            return
        if self.path == "/status":
            if not self.plane.verify_signature(b"", self.headers.get("X-YTEAM-Signature", ""), "GET", self.path):
                self._json(401, {"error": "invalid signature"})
                return
            self._json(200, self.plane.runtime.snapshot())
            return
        self._json(404, {"error": "not found"})

    def do_POST(self) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        if length > 65536:
            self._json(413, {"error": "request too large"})
            return
        body = self.rfile.read(length)
        if not self.plane.verify_signature(body, self.headers.get("X-YTEAM-Signature", ""), "POST", self.path):
            self._json(401, {"error": "invalid signature"})
            return
        provider = self.path.strip("/").split("/", 1)[-1] or "webhook"
        try:
            payload = json.loads(body)
        except json.JSONDecodeError:
            payload = urllib.parse.parse_qs(body.decode("utf-8", "replace"))
        if provider == "whatsapp" and isinstance(payload, dict) and "From" in payload:
            actor = str(payload.get("From", [""])[0] if isinstance(payload.get("From"), list) else payload.get("From", ""))
            text = str(payload.get("Body", [""])[0] if isinstance(payload.get("Body"), list) else payload.get("Body", ""))
        else:
            actor = str(payload.get("actor", "")) if isinstance(payload, dict) else ""
            text = str(payload.get("text", "")) if isinstance(payload, dict) else ""
        result = self.plane.execute(provider, actor, text)
        self._json(200, {"ok": True, "reply": result})

    def _json(self, status: int, value: object) -> None:
        data = json.dumps(value, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *_args: object) -> None:
        return


def serve(runtime, root: Path, host: str = "127.0.0.1", port: int = 8787, executor: Callable[[str], None] | None = None) -> ThreadingHTTPServer:
    plane = ControlPlane(runtime, root)
    plane.executor = executor
    Handler.plane = plane
    server = ThreadingHTTPServer((host, port), Handler)
    plane.start_telegram()
    return server


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default=os.environ.get("YTEAM_CONTROL_HOST", "127.0.0.1"))
    parser.add_argument("--port", type=int, default=int(os.environ.get("YTEAM_CONTROL_PORT", "8787")))
    args = parser.parse_args()
    from yteam_runtime import YteamRuntime

    runtime = YteamRuntime(Path(__file__).resolve().parents[1])
    if not os.environ.get("YTEAM_CONTROL_SECRET"):
        raise SystemExit("YTEAM_CONTROL_SECRET is required; refusing unsigned remote control")
    server = serve(runtime, runtime.root, args.host, args.port)
    print(f"YTEAM control plane listening on {args.host}:{args.port}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
