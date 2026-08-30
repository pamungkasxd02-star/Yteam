"""Optional Camoufox observer for LocalSolver authorized recon.

Camoufox is used to observe browser-visible responses and challenge markers in
an isolated context. This adapter never solves challenges, changes fingerprints,
rotates identities/proxies, or attempts WAF evasion.
"""

from __future__ import annotations

import asyncio
import json
import time
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

from .detector import LocalSolver, LocalSolverDecision


@dataclass(frozen=True)
class CamoufoxConfig:
    target: str
    output: Path
    headless: bool = True
    timeout_ms: int = 12_000
    max_responses: int = 80
    max_body_bytes: int = 131_072
    rate: float = 1.0


def same_origin(url: str, target: str) -> bool:
    left, right = urlsplit(url), urlsplit(target)
    return left.scheme in {"http", "https"} and left.scheme == right.scheme and (left.hostname or "").lower() == (right.hostname or "").lower() and (left.port or (443 if left.scheme == "https" else 80)) == (right.port or (443 if right.scheme == "https" else 80))


def safe_url(value: str) -> str:
    try:
        parsed = urlsplit(value)
    except ValueError:
        return "<REDACTED_URL>"
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return "<REDACTED_URL>"
    sensitive = {"access_token", "refresh_token", "id_token", "token", "session", "session_id", "code", "state", "nonce", "password", "secret", "api_key", "signature", "sig"}
    query = [(key, "<REDACTED>") if key.lower().replace("-", "_") in sensitive else (key, item) for key, item in parse_qsl(parsed.query, keep_blank_values=True)]
    return urlunsplit((parsed.scheme, parsed.hostname or "", parsed.path, urlencode(query), ""))


def _safe_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, default=str) + "\n", encoding="utf-8")


def _load_camoufox() -> Any:
    try:
        from camoufox.async_api import AsyncCamoufox
    except ImportError:
        return None
    return AsyncCamoufox


async def _run_async(config: CamoufoxConfig) -> dict[str, object]:
    parsed = urlsplit(config.target)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise ValueError("Camoufox target must be an HTTP(S) URL")
    factory = _load_camoufox()
    if factory is None:
        result = {"status": "unavailable", "engine": "localsolver-camoufox", "target": safe_url(config.target), "reason": "Camoufox is not installed; use the documented optional dependency installation.", "action": "manual_review", "policy": "isolated observation only; no challenge solving or evasion"}
        _safe_json(config.output / "localsolver.json", result)
        return result
    config.output.mkdir(parents=True, exist_ok=True)
    solver = LocalSolver(config.rate)
    responses: list[dict[str, object]] = []
    notes: list[str] = []
    halted = False
    navigation_error = False
    async with factory(headless=config.headless) as browser:
        page = await browser.new_page()
        tasks: list[asyncio.Task[None]] = []

        async def capture(response: Any) -> None:
            nonlocal halted
            if halted or len(responses) >= config.max_responses or not same_origin(str(response.url), config.target):
                return
            try:
                headers = {str(key): str(value) for key, value in (await response.all_headers()).items()}
            except Exception:  # noqa: BLE001
                headers = {}
            body = ""
            content_type = headers.get("content-type", "")
            if any(value in content_type.lower() for value in ("text", "json", "javascript")):
                try:
                    body = (await response.body())[:config.max_body_bytes].decode("utf-8", errors="replace")
                except Exception:  # noqa: BLE001
                    body = ""
            decision: LocalSolverDecision = solver.inspect(headers, body, int(response.status))
            responses.append({"url": safe_url(str(response.url)), "status": int(response.status), "content_type": content_type, "decision": {**asdict(decision), "evidence": list(decision.evidence)}})
            if decision.action in {"stop", "manual_review"}:
                halted = True
                notes.append(f"LocalSolver {decision.action}: {decision.gate} at {safe_url(str(response.url))}")

        def on_response(response: Any) -> None:
            if not halted and len(tasks) < config.max_responses:
                tasks.append(asyncio.create_task(capture(response)))

        page.on("response", on_response)
        solver.wait()
        started = time.monotonic()
        try:
            await page.goto(config.target, wait_until="domcontentloaded", timeout=config.timeout_ms)
            elapsed_ms = int((time.monotonic() - started) * 1000)
        except Exception as error:  # noqa: BLE001
            navigation_error = True
            elapsed_ms = int((time.monotonic() - started) * 1000)
            notes.append(f"LocalSolver navigation error: {type(error).__name__}")
        if not halted:
            await page.wait_for_timeout(min(1_000, config.timeout_ms // 4))
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)
        await page.close()
    result = {"status": "degraded" if navigation_error and not responses else "completed", "engine": "localsolver-camoufox", "target": safe_url(config.target), "headless": config.headless, "elapsed_ms": elapsed_ms, "response_count": len(responses), "halted": halted, "responses": responses, "localsolver": solver.summary(), "notes": notes, "policy": "isolated browser observation only; no challenge solving or evasion"}
    _safe_json(config.output / "localsolver.json", result)
    (config.output / "localsolver_notes.md").write_text("# LocalSolver Camoufox Notes\n\n" + "\n".join(f"- {note}" for note in notes) + "\n\n## Non-claims\n\n- LocalSolver observation is not a bypass.\n- Challenge markers are recorded as blocker/manual-review state.\n", encoding="utf-8")
    return result


def run_camoufox(config: CamoufoxConfig) -> dict[str, object]:
    try:
        return asyncio.run(_run_async(config))
    except ValueError:
        raise
    except Exception as error:  # noqa: BLE001
        result = {"status": "unavailable", "engine": "localsolver-camoufox", "target": safe_url(config.target), "reason": f"Camoufox observation could not start: {type(error).__name__}.", "action": "manual_review", "policy": "observation-only; no challenge solving or evasion"}
        _safe_json(config.output / "localsolver.json", result)
        return result
