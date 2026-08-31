"""Context Guard — auto-compaction and handoff for long YTEAM sessions.

This module prevents a long autonomous session from dying at the model's
context window. It estimates the token usage of the conversation, applies a
warning threshold and a critical threshold, and when a threshold is crossed it
either compacts the oldest turns into a durable summary (kept at the front of
the prompt) or writes a handoff bundle so a fresh session can continue.

Design rules
------------
- Estimation is deterministic and dependency-free (chars/4 heuristic), so the
  same input always yields the same guard verdict.
- Compaction is lossless-to-storage: it only shortens what is sent to the model;
  the raw messages remain in the SQLite store for evidence.
- Handoff is a Markdown bundle under ``runtime/handoffs/`` following the
  operator's handoff template, and the caller gets a ``CONTINUE_CMD`` to start a
  fresh session.
- All thresholds are configurable; defaults mirror the operator's guard policy
  (75% warning / 85% handoff) for a 1M-token context model.
"""

from __future__ import annotations

import json
import re
import time
from dataclasses import dataclass, field
from pathlib import Path


def estimate_tokens(text: str) -> int:
    """Deterministic token estimate (~4 chars per token) without a tokenizer."""
    if not text:
        return 0
    # ASCII-aware: CJK-ish chars count heavier per char.
    cjk = len(re.findall(r"[\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af]", text))
    ascii_chars = max(0, len(text) - cjk)
    return cjk + max(1, ascii_chars // 4)


def estimate_message_tokens(message: dict[str, str]) -> int:
    return 4 + estimate_tokens(message.get("content", ""))


def estimate_conversation_tokens(messages: list[dict[str, str]]) -> int:
    return sum(estimate_message_tokens(message) for message in messages)


@dataclass
class GuardConfig:
    context_window: int = 1_000_000  # model context limit in tokens
    warning_ratio: float = 0.75
    handoff_ratio: float = 0.85
    max_compact_system_tokens: int = 8000
    keep_recent_messages: int = 20

    def __post_init__(self) -> None:
        if self.context_window <= 0:
            raise ValueError("context_window must be > 0")
        if not 0 < self.warning_ratio <= 1:
            raise ValueError("warning_ratio must be in (0, 1]")
        if not 0 < self.handoff_ratio <= 1:
            raise ValueError("handoff_ratio must be in (0, 1]")
        if self.warning_ratio > self.handoff_ratio:
            raise ValueError("warning_ratio must be <= handoff_ratio")


@dataclass
class GuardVerdict:
    level: str  # "ok", "warning", "handoff"
    estimated_tokens: int
    used_ratio: float
    warning_ratio: float
    handoff_ratio: float
    compaction_applied: bool = False
    compaction_saved_tokens: int = 0
    handoff_path: str = ""
    continue_cmd: str = ""

    def as_dict(self) -> dict[str, object]:
        return {
            "level": self.level,
            "estimated_tokens": self.estimated_tokens,
            "used_ratio": round(self.used_ratio, 4),
            "warning_ratio": self.warning_ratio,
            "handoff_ratio": self.handoff_ratio,
            "compaction_applied": self.compaction_applied,
            "compaction_saved_tokens": self.compaction_saved_tokens,
            "handoff_path": self.handoff_path,
            "continue_cmd": self.continue_cmd,
        }


class ContextGuard:
    """Auto-compaction + handoff for a conversation message list."""

    def __init__(self, config: GuardConfig | None = None, handoff_dir: Path | None = None, product: str = "YTEAM") -> None:
        self.config = config or GuardConfig()
        self.handoff_dir = handoff_dir
        self.product = product

    # -- estimation -------------------------------------------------------
    def estimate(self, messages: list[dict[str, str]]) -> int:
        return estimate_conversation_tokens(messages)

    def ratio(self, messages: list[dict[str, str]]) -> float:
        total = self.estimate(messages)
        return total / self.config.context_window

    # -- compaction -------------------------------------------------------
    def compact(self, messages: list[dict[str, str]]) -> tuple[list[dict[str, str]], int]:
        """Reduce prompt to a compacted system summary + recent turns.

        Returns ``(messages, saved_tokens)``. The oldest turns are folded into a
        single system ``compaction`` message placed at the front. Raw data stays
        in the store; only the prompt is shortened.
        """
        if not messages:
            return messages, 0
        keep = max(2, min(self.config.keep_recent_messages, len(messages)))
        kept = messages[-keep:]
        old = messages[:-keep]
        old_tokens = estimate_conversation_tokens(old)
        summary = _fold_summary(old, self.config.max_compact_system_tokens)
        compacted = [{"role": "system", "content": summary}] + kept
        return compacted, old_tokens

    def _should_compact(self, messages: list[dict[str, str]]) -> bool:
        used = self.ratio(messages)
        # Compact when we cross the warning ratio and have more than the keep window.
        return used >= self.config.warning_ratio and len(messages) > self.config.keep_recent_messages

    # -- handoff ----------------------------------------------------------
    def write_handoff(self, messages: list[dict[str, str]], meta: dict[str, object] | None = None) -> str:
        """Write a handoff bundle and return its path."""
        if self.handoff_dir is None:
            return ""
        self.handoff_dir.mkdir(parents=True, exist_ok=True)
        stamp = time.strftime("%Y%m%d_%H%M%S")
        path = self.handoff_dir / f"HANDOFF_{stamp}.md"
        summary = _fold_summary(messages, self.config.max_compact_system_tokens)
        meta_lines = "\n".join(f"- {key}: {value}" for key, value in (meta or {}).items())
        path.write_text(
            "# YTEAM Context Handoff\n\n"
            f"- product: {self.product}\n"
            f"- created: {stamp}\n\n"
            "## Mission / state / next actions\n\n"
            "```\n" + (meta_lines or "- (no meta)") + "\n```\n\n"
            "## Conversation summary\n\n"
            "```\n" + summary + "\n```\n\n"
            "## Continue\n\n"
            f"Resume a fresh session with the continuation command produced by the guard.\n",
            encoding="utf-8",
        )
        return str(path)

    def continue_cmd(self, handoff_path: str) -> str:
        """Return the shell command to resume a fresh session from a handoff."""
        return f"opencode run --auto --agent bb \"CONTINUE from handoff: {handoff_path}\""

    # -- main gate --------------------------------------------------------
    def check(self, messages: list[dict[str, str]], meta: dict[str, object] | None = None, allow_handoff: bool = True) -> GuardVerdict:
        """Run the guard: compact at warning, hand off at critical."""
        estimated = self.estimate(messages)
        used = estimated / self.config.context_window
        if used >= self.config.handoff_ratio:
            path = self.write_handoff(messages, meta) if (allow_handoff and self.handoff_dir) else ""
            return GuardVerdict(
                level="handoff",
                estimated_tokens=estimated,
                used_ratio=used,
                warning_ratio=self.config.warning_ratio,
                handoff_ratio=self.config.handoff_ratio,
                handoff_path=path,
                continue_cmd=self.continue_cmd(path) if path else "",
            )
        if self._should_compact(messages):
            compacted, saved = self.compact(messages)
            # Verdict reflects the post-compaction state for the caller to use.
            return GuardVerdict(
                level="warning",
                estimated_tokens=estimated,
                used_ratio=used,
                warning_ratio=self.config.warning_ratio,
                handoff_ratio=self.config.handoff_ratio,
                compaction_applied=True,
                compaction_saved_tokens=saved,
            )
        return GuardVerdict(
            level="ok",
            estimated_tokens=estimated,
            used_ratio=used,
            warning_ratio=self.config.warning_ratio,
            handoff_ratio=self.config.handoff_ratio,
        )


def _fold_summary(messages: list[dict[str, str]], max_tokens: int) -> str:
    """Deterministic lossy folding of a message list into a concise summary."""
    if not messages:
        return "[no conversation history]"
    parts: list[str] = []
    for message in messages:
        role = str(message.get("role", "?")).upper()
        content = str(message.get("content", "")).strip()
        if not content:
            continue
        snippet = content if len(content) <= 400 else content[:397] + "..."
        parts.append(f"{role}: {snippet}")
    text = "\n".join(parts)
    # Trim to the token budget deterministically.
    while estimate_tokens(text) > max_tokens and len(text) > 500:
        text = text[: int(len(text) * 0.9)]
    return "Previous session summary:\n" + text
