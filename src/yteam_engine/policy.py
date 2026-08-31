"""Policy engine for YTEAM: schema-validated, deny-by-default resolution.

A policy is a small JSON/YAML document that declares the maximum allowed
side-effect class and the per-target request budget. Resolution is
deny-by-default: an absent rule denies. Components (graph, scheduler, planner)
call ``assert_effect`` / ``budget_for`` before acting.

The schema is intentionally strict so an invalid policy fails at load time, not
mid-run. YAML support is optional; the core validates pure JSON documents so it
runs even without PyYAML installed.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

SIDE_EFFECT_RANK = {
    "read": 1,
    "analyze": 2,
    "external": 3,
    "write": 4,
    "report": 5,
}

ALLOWED_KEYS = {
    "schema_version",
    "default_side_effect",
    "per_target_rate",
    "targets",
}

ALLOWED_TARGET_KEYS = {
    "allowed_effects",
    "rate_per_second",
    "notes",
}

MAX_RATE = 25.0  # hard cap, matching the operator's safe request budget


class PolicyViolation(Exception):
    """Raised when an action exceeds the active policy."""


class PolicyError(ValueError):
    """Raised when a policy document is malformed."""


@dataclass(frozen=True)
class ResolvedBudget:
    effect_rank: int
    allowed_effects: frozenset[str]
    rate_per_second: float


def _validate_schema(document: Any) -> dict[str, Any]:
    if not isinstance(document, dict):
        raise PolicyError("policy must be a JSON/YAML object")
    for key in document:
        if key not in ALLOWED_KEYS:
            raise PolicyError(f"unknown policy key: {key!r}")
    version = document.get("schema_version", 1)
    if version != 1:
        raise PolicyError(f"unsupported policy schema_version: {version}")
    default = document.get("default_side_effect", "read")
    if default not in SIDE_EFFECT_RANK:
        raise PolicyError(f"default_side_effect must be one of {sorted(SIDE_EFFECT_RANK)}")
    rate = document.get("per_target_rate", 1.0)
    if not isinstance(rate, (int, float)) or rate < 0 or rate > MAX_RATE:
        raise PolicyError(f"per_target_rate out of bounds [0, {MAX_RATE}]: {rate!r}")
    targets = document.get("targets", {})
    if not isinstance(targets, dict):
        raise PolicyError("targets must be an object keyed by target pattern")
    for pattern, spec in targets.items():
        if not isinstance(pattern, str) or not pattern.strip():
            raise PolicyError("target pattern must be a non-empty string")
        if not isinstance(spec, dict):
            raise PolicyError(f"target {pattern!r} spec must be an object")
        for key in spec:
            if key not in ALLOWED_TARGET_KEYS:
                raise PolicyError(f"target {pattern!r} has unknown key {key!r}")
        effects = spec.get("allowed_effects")
        if effects is not None:
            if not isinstance(effects, (list, tuple, set)):
                raise PolicyError(f"target {pattern!r} allowed_effects must be a list")
            for effect in effects:
                if effect not in SIDE_EFFECT_RANK:
                    raise PolicyError(f"target {pattern!r} has unknown effect {effect!r}")
        t_rate = spec.get("rate_per_second")
        if t_rate is not None and (not isinstance(t_rate, (int, float)) or t_rate < 0 or t_rate > MAX_RATE):
            raise PolicyError(f"target {pattern!r} rate_per_second out of bounds")
    return document


def _pattern_match(target: str, pattern: str) -> bool:
    value = target.lower()
    parsed = urlparse(value if "://" in value else f"https://{value}")
    host = (parsed.hostname or "").lower()
    pattern = pattern.strip().lower()
    if pattern == "*":
        return True
    if pattern.startswith("*."):
        return host == pattern[2:] or host.endswith(pattern[1:])
    if pattern.startswith("regex:"):
        try:
            return re.search(pattern[6:], value) is not None
        except re.error:
            return False
    return host == pattern or value == pattern


class Policy:
    """Load, validate, and enforce a YTEAM policy document."""

    def __init__(self, document: dict[str, Any]) -> None:
        self.document = _validate_schema(document)
        self.default_effect = str(self.document.get("default_side_effect", "read"))
        self.default_rate = float(self.document.get("per_target_rate", 1.0))
        self.targets = dict(self.document.get("targets", {}))

    @classmethod
    def from_file(cls, path: Path) -> "Policy":
        if not path.exists():
            raise PolicyError(f"policy file not found: {path}")
        text = path.read_text(encoding="utf-8", errors="replace")
        if path.suffix.lower() in {".yaml", ".yml"}:
            try:
                import yaml
            except ImportError as error:  # pragma: no cover - optional dep
                raise PolicyError("YAML policy requires PyYAML; use a JSON policy instead") from error
            document = yaml.safe_load(text) or {}
        else:
            try:
                document = json.loads(text)
            except json.JSONDecodeError as error:
                raise PolicyError(f"invalid JSON policy: {error}") from error
        return cls(document)

    @classmethod
    def default(cls) -> "Policy":
        """A strict read-only, low-rate, deny-everything-else policy."""
        return cls({
            "schema_version": 1,
            "default_side_effect": "read",
            "per_target_rate": 1.0,
            "targets": {},
        })

    def resolve(self, target: str) -> ResolvedBudget:
        """Return the resolved budget for a target (most specific match wins)."""
        effect_rank = SIDE_EFFECT_RANK[self.default_effect]
        rate = self.default_rate
        allowed_effects: frozenset[str] | None = None
        best_score = -1
        for pattern, spec in self.targets.items():
            if not _pattern_match(target, pattern):
                continue
            score = _pattern_score(pattern)
            if score <= best_score:
                continue
            best_score = score
            if "rate_per_second" in spec:
                rate = float(spec["rate_per_second"])
            if "allowed_effects" in spec:
                effects = {str(effect) for effect in spec["allowed_effects"]}
                rank = max(SIDE_EFFECT_RANK[effect] for effect in effects) if effects else 0
                effect_rank = rank
                allowed_effects = frozenset(effects)
        if allowed_effects is None:
            allowed_effects = frozenset(
                effect for effect, rank in SIDE_EFFECT_RANK.items() if rank <= effect_rank
            )
        return ResolvedBudget(effect_rank, allowed_effects, rate)

    def assert_effect(self, side_effect: str, target: str) -> None:
        """Raise PolicyViolation if ``side_effect`` is not allowed for ``target``."""
        budget = self.resolve(target)
        rank = SIDE_EFFECT_RANK.get(side_effect)
        if rank is None:
            raise PolicyViolation(f"unknown side_effect {side_effect!r} for {target!r}")
        if rank > budget.effect_rank or side_effect not in budget.allowed_effects:
            raise PolicyViolation(
                f"side_effect {side_effect!r} not allowed for {target!r} "
                f"(max effect rank {budget.effect_rank}, allowed {sorted(budget.allowed_effects)})"
            )

    def budget_for(self, target: str) -> ResolvedBudget:
        return self.resolve(target)

    def to_dict(self) -> dict[str, object]:
        return {
            "default_side_effect": self.default_effect,
            "per_target_rate": self.default_rate,
            "targets": self.targets,
        }


def _pattern_score(pattern: str) -> int:
    """Score specificity so ``api.example.com`` outranks ``*.example.com``."""
    if pattern == "*":
        return 0
    if pattern.startswith("regex:"):
        return 3
    if pattern.startswith("*."):
        return 2
    return 4
