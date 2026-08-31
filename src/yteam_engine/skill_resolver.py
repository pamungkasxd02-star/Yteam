"""Plugin/skill runtime resolver for YTEAM.

The skill registry is static metadata. This resolver turns it into a runtime
service: on-demand body loading with an LRU cache, dependency injection
(interfaces/tools injected per skill), a load policy that refuses quarantined
bodies, and versioning so a changed SKILL.md invalidates the cache.

It does not execute attacker payloads; it only resolves reviewed, policy-bound
skill content for the planner/worker to reason over.
"""

from __future__ import annotations

import hashlib
import threading
from collections import OrderedDict
from typing import Any, Callable

from .policy import Policy


class SkillResolutionError(Exception):
    """Raised when a skill cannot be resolved under the active policy."""


class SkillResolver:
    """Caches and injects skill bodies under a policy + DI container."""

    def __init__(self, policy: Policy, registry_fn: Callable[[], list[dict[str, object]]], get_fn: Callable[..., dict[str, object]], cache_size: int = 32, allow_controlled: bool = False) -> None:
        self.policy = policy
        self._registry_fn = registry_fn
        self._get_fn = get_fn
        self.cache_size = max(1, int(cache_size))
        self.allow_controlled = bool(allow_controlled)
        self._cache: OrderedDict[str, dict[str, object]] = OrderedDict()
        self._lock = threading.RLock()
        self._interfaces: dict[str, Any] = {}

    # -- dependency injection ---------------------------------------------
    def bind(self, name: str, value: Any) -> "SkillResolver":
        with self._lock:
            self._interfaces[name] = value
        return self

    def _dependencies_for(self, skill: dict[str, object]) -> dict[str, Any]:
        requested = skill.get("metadata", {}).get("deps", [])
        if not isinstance(requested, list):
            requested = []
        return {str(name): self._interfaces.get(name) for name in requested if str(name) in self._interfaces}

    # -- cache ------------------------------------------------------------
    def _cache_key(self, skill: dict[str, object]) -> str:
        digest = hashlib.sha256(f"{skill.get('name')}|{skill.get('content_sha256', '')}".encode("utf-8")).hexdigest()[:16]
        return f"{skill.get('name')}:{digest}"

    def _put(self, key: str, value: dict[str, object]) -> None:
        with self._lock:
            self._cache[key] = value
            self._cache.move_to_end(key)
            while len(self._cache) > self.cache_size:
                self._cache.popitem(last=False)

    def _get_cached(self, key: str) -> dict[str, object] | None:
        with self._lock:
            if key in self._cache:
                self._cache.move_to_end(key)
                return self._cache[key]
        return None

    # -- resolution -------------------------------------------------------
    def resolve(self, skill_name: str, section: str = "") -> dict[str, object]:
        """Return skill content with injected dependencies, or raise if blocked."""
        items = self._registry_fn()
        match = next((item for item in items if str(item["name"]).lower() == skill_name.strip().lower()), None)
        if match is None:
            raise SkillResolutionError(f"skill not found: {skill_name}")
        risk = str(match.get("risk", "safe_reference"))
        if risk == "quarantined":
            raise SkillResolutionError(f"skill {skill_name!r} is quarantined; body loading disabled")
        if risk == "controlled" and not self.allow_controlled:
            # controlled content is allowed to load metadata but not run; return
            # the body only when explicitly allowed.
            raise SkillResolutionError(f"skill {skill_name!r} is controlled; set allow_controlled to load")
        key = self._cache_key(match) + f"|{section}"
        cached = self._get_cached(key)
        if cached is not None:
            return cached
        try:
            loaded = self._get_fn(items, skill_name, section, self.allow_controlled)
        except Exception as error:  # noqa: BLE001
            raise SkillResolutionError(f"failed to load skill {skill_name!r}: {error}") from error
        if not isinstance(loaded, dict):
            raise SkillResolutionError(f"skill {skill_name!r} returned a non-dict body")
        deps = self._dependencies_for(match)
        if deps:
            loaded = {**loaded, "_injected_deps": deps}
        self._put(key, loaded)
        return loaded

    def prefetch(self, skill_names: Iterable[str], section: str = "") -> dict[str, str]:
        """Eagerly resolve a set of skills, returning per-skill status."""
        status: dict[str, str] = {}
        for name in skill_names:
            try:
                self.resolve(name, section)
                status[name] = "loaded"
            except (SkillResolutionError, Exception):  # noqa: BLE001
                status[name] = "blocked"
        return status

    def cache_stats(self) -> dict[str, object]:
        with self._lock:
            return {"size": len(self._cache), "capacity": self.cache_size, "keys": list(self._cache.keys())}

    def invalidate(self) -> None:
        with self._lock:
            self._cache.clear()
