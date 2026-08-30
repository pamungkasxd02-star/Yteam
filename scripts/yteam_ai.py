#!/usr/bin/env python3
"""Native OpenAI-compatible streaming client used by YTEAM."""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from collections.abc import Iterator
from dataclasses import dataclass


@dataclass(frozen=True)
class ChatRequest:
    model: str
    messages: list[dict[str, str]]
    temperature: float = 0.2
    max_tokens: int = 4096


def stream_chat(config: dict[str, str], messages: list[dict[str, str]]) -> Iterator[str]:
    """Yield text deltas from an OpenAI-compatible chat-completions endpoint."""
    request_data = ChatRequest(config["model"], messages)
    body = json.dumps({
        "model": request_data.model,
        "messages": request_data.messages,
        "temperature": request_data.temperature,
        "max_tokens": request_data.max_tokens,
        "stream": True,
    }).encode("utf-8")
    headers = {"Accept": "text/event-stream", "Content-Type": "application/json", "User-Agent": "YTEAM/1.0"}
    if config.get("api_key"):
        headers["Authorization"] = f"Bearer {config['api_key']}"
    request = urllib.request.Request(f"{config['base_url'].rstrip('/')}/chat/completions", data=body, method="POST", headers=headers)
    try:
        response = urllib.request.urlopen(request, timeout=float(config.get("timeout", "300")))
    except urllib.error.HTTPError as error:
        detail = error.read(800).decode("utf-8", "replace")
        raise RuntimeError(f"model provider returned HTTP {error.code}: {detail}") from error
    with response:
        for raw_line in response:
            line = raw_line.decode("utf-8", "replace").strip()
            if not line.startswith("data: "):
                continue
            payload = line[6:]
            if payload == "[DONE]":
                break
            try:
                chunk = json.loads(payload)
            except json.JSONDecodeError:
                continue
            for choice in chunk.get("choices", []):
                text = ((choice.get("delta") or {}).get("content"))
                if isinstance(text, str) and text:
                    yield text


def complete_chat(config: dict[str, str], messages: list[dict[str, str]]) -> str:
    """Collect a streaming completion for callers that do not need live output."""
    return "".join(stream_chat(config, messages))
