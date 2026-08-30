"""Optional Camoufox adapter for authorized Botterdop browser QA.

This module is deliberately detection-only. It does not solve challenges,
spoof fingerprints, rotate identities, mutate WAF headers, or bypass gates.
Camoufox is used as an isolated browser client so Botterdop can observe the
same challenge page a normal browser would receive.
"""

from __future__ import annotations

import asyncio
import json
import time
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

from .detector import Botterdop, BotterdopDecision


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
    left = urlsplit(url)
    right = urlsplit(target)
    return left.scheme in {"http", "https"} and left.scheme == right.scheme and (left.hostname or "").lower() == (right.hostname or "").lower() and (left.port or (443 if left.scheme == "https" else 80)) == (right.port or (443 if right.scheme == "https" else 80))


SENSITIVE_QUERY_NAMES = {
    "access_token", "refresh_token", "id_token", "token", "session", "session_id",
    "code", "state", "nonce", "password", "secret", "api_key", "signature", "sig",
}


def safe_url(value: str) -> str:
    """Preserve route shape while removing credential-bearing query values."""
    try:
        parsed = urlsplit(value)
    except ValueError:
        return "<REDACTED_URL>"
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        return "<REDACTED_URL>"
    query = [(key, "<REDACTED>") if key.lower().replace("-", "_") in SENSITIVE_QUERY_NAMES else (key, item) for key, item in parse_qsl(parsed.query, keep_blank_values=True)]
    return urlunsplit((parsed.scheme, parsed.hostname or "", parsed.path, urlencode(query), ""))


def _safe_json(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, default=str) + "\n", encoding="utf-8")


def _decision_dict(decision: BotterdopDecision) -> dict[str, object]:
    value = asdict(decision)
    value["evidence"] = list(decision.evidence)
    return value


def _load_camoufox() -> Any:
    try:
        from camoufox.async_api import AsyncCamoufox
    except ImportError:
        return None
    return AsyncCamoufox


async def _run_async(config: CamoufoxConfig) -> dict[str, object]:
    parsed_target = urlsplit(config.target)
    if parsed_target.scheme not in {"http", "https"} or not parsed_target.hostname:
        raise ValueError("Camoufox target must be an HTTP(S) URL")
    safe_target = safe_url(config.target)
    factory = _load_camoufox()
    if factory is None:
        result = {
            "status": "unavailable",
            "engine": "camoufox",
            "target": safe_target,
            "reason": "Camoufox is not installed; use the documented optional installation or native HTTP Botterdop detection.",
            "action": "manual_review",
            "policy": "detection-only; no challenge solving or evasion",
        }
        _safe_json(config.output / "camoufox.json", result)
        return result

    config.output.mkdir(parents=True, exist_ok=True)
    governor = Botterdop(config.rate)
    responses: list[dict[str, object]] = []
    notes: list[str] = []
    halted = False
    navigation_error = False

    async with factory(headless=config.headless) as browser:
        page = await browser.new_page()

        async def capture(response: Any) -> None:
            nonlocal halted
            if halted or len(responses) >= config.max_responses:
                return
            response_url = str(response.url)
            if not same_origin(response_url, config.target):
                return
            try:
                headers = {str(key): str(value) for key, value in (await response.all_headers()).items()}
            except Exception:  # noqa: BLE001 - browser adapters must not break the run
                headers = {}
            body = ""
            content_type = headers.get("content-type", "")
            if "text" in content_type.lower() or "json" in content_type.lower() or "javascript" in content_type.lower():
                try:
                    body = (await response.body())[: config.max_body_bytes].decode("utf-8", errors="replace")
                except Exception:  # noqa: BLE001
                    body = ""
            decision = governor.inspect(headers, body, int(response.status))
            responses.append({
                "url": safe_url(response_url),
                "status": int(response.status),
                "content_type": content_type,
                "decision": _decision_dict(decision),
            })
            if decision.action in {"stop", "manual_review"}:
                halted = True
                notes.append(f"Botterdop {decision.action}: {decision.gate} detected at {safe_url(response_url)}")

        response_tasks: list[asyncio.Task[None]] = []

        def on_response(response: Any) -> None:
            if not halted and len(response_tasks) < config.max_responses:
                response_tasks.append(asyncio.create_task(capture(response)))

        page.on("response", on_response)
        governor.wait()
        started = time.monotonic()
        try:
            await page.goto(config.target, wait_until="domcontentloaded", timeout=config.timeout_ms)
            elapsed_ms = int((time.monotonic() - started) * 1000)
        except Exception as error:  # noqa: BLE001
            navigation_error = True
            elapsed_ms = int((time.monotonic() - started) * 1000)
            notes.append(f"Camoufox navigation error: {type(error).__name__}")
        if not halted:
            await page.wait_for_timeout(min(1_000, config.timeout_ms // 4))
        if response_tasks:
            await asyncio.gather(*response_tasks, return_exceptions=True)
        await page.close()

    result = {
        "status": "degraded" if navigation_error and not responses else "completed",
        "engine": "camoufox",
        "target": safe_target,
        "headless": config.headless,
        "elapsed_ms": elapsed_ms,
        "response_count": len(responses),
        "halted": halted,
        "responses": responses,
        "botterdop": governor.summary(),
        "notes": notes,
        "policy": "isolated browser observation only; no challenge solving or evasion",
    }
    _safe_json(config.output / "camoufox.json", result)
    (config.output / "camoufox_notes.md").write_text(
        "# Botterdop Camoufox Notes\n\n"
        + "\n".join(f"- {note}" for note in notes)
        + "\n\n## Non-claims\n\n- Camoufox observation is not a bypass.\n- A challenge is recorded as a blocker/manual-review state.\n",
        encoding="utf-8",
    )
    return result


def run_camoufox(config: CamoufoxConfig) -> dict[str, object]:
    """Run one bounded, same-origin Camoufox observation session."""
    try:
        return asyncio.run(_run_async(config))
    except ValueError:
        raise
    except Exception as error:  # noqa: BLE001 - optional browser must degrade safely
        result = {
            "status": "unavailable",
            "engine": "camoufox",
            "target": safe_url(config.target),
            "reason": f"Camoufox observation could not start: {type(error).__name__}.",
            "action": "manual_review",
            "policy": "detection-only; no challenge solving or evasion",
        }
        _safe_json(config.output / "camoufox.json", result)
        return result
