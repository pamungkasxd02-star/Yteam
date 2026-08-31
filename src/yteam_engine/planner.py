"""Adaptive recon/attack planner for YTEAM.

The planner is a self-tuning state machine. Given a target and the live signals
observed so far (status codes, reflected markers, WAF/gate indicators, object
reference shapes), it selects the next most promising technique. It does not
execute anything itself; it emits a plan the caller (durable worker / hunt
pipeline) executes under scope and rate policy.

Self-tuning: each completed step yields a reward (positive signal found, or
explicit non-claim), and the planner adjusts per-technique weights so the
techniques that actually produced signal on this target rank higher next turn.
"""

from __future__ import annotations

import math
import threading
from dataclasses import dataclass, field
from typing import Iterable

from .policy import Policy

# Technique catalogue: id -> (category, description)
TECHNIQUES: dict[str, tuple[str, str]] = {
    "recon.robots_sitemap": ("recon", "read robots.txt / sitemap.xml for route hints"),
    "recon.js_bundle": ("recon", "download and grep first-party JS bundles for endpoints/keys"),
    "recon.crt_sh": ("recon", "certificate transparency subdomain discovery"),
    "recon.archive": ("recon", "passive archive URL harvesting"),
    "map.openapi": ("mapping", "probe /openapi.json, /swagger, /v3/api-docs"),
    "map.graphql_introspect": ("mapping", "probe GraphQL introspection and field suggestion"),
    "auth.unauth_object": ("auth", "probe documented object route with synthetic id"),
    "auth.method_override": ("auth", "probe HTTP verb tampering on protected routes"),
    "auth.jwt_none": ("auth", "test alg:none / weak-signature token handling"),
    "data.idor_read": ("data", "swap object id across owned identities"),
    "data.mass_assignment": ("data", "send extra privileged fields on profile update"),
    "data.ssrf_meta": ("data", "probe server-side fetch for internal/metadata"),
    "inject.sqli_bool": ("injection", "boolean-based SQLi probes on search/filter"),
    "inject.xss_reflected": ("injection", "reflected XSS canary in reflected params"),
    "inject.ssti": ("injection", "template-injection polyglot probes"),
    "logic.race_coupon": ("logic", "parallel redeem to detect double-spend"),
    "logic.csrf_state": ("logic", "state-change without anti-CSRF token"),
    "client.cors_creds": ("client", "credentialed cross-origin read probe"),
}

# Default per-category starting weight; read/analyze techniques are cheap and
# safe, write/inject techniques are gated by policy and start lower.
DEFAULT_WEIGHTS: dict[str, float] = {
    "recon": 1.0,
    "mapping": 0.9,
    "auth": 0.8,
    "data": 0.7,
    "injection": 0.5,
    "logic": 0.4,
    "client": 0.4,
}


@dataclass
class StepResult:
    technique: str
    reward: float  # positive => signal found; negative => explicit non-claim
    note: str = ""
    status: str = "completed"  # completed, blocked, failed


@dataclass
class PlanStep:
    technique: str
    category: str
    description: str
    side_effect: str
    weight: float


@dataclass
class PlannerState:
    target: str
    round: int = 0
    rewards: dict[str, float] = field(default_factory=dict)
    completed_techniques: list[str] = field(default_factory=list)
    blocked_techniques: list[str] = field(default_factory=list)


class AdaptivePlanner:
    """Selects and adaptively re-weights techniques for one target."""

    def __init__(self, policy: Policy, learning_rate: float = 0.2, plan_size: int = 5) -> None:
        self.policy = policy
        self.learning_rate = max(0.0, min(1.0, float(learning_rate)))
        self.plan_size = max(1, int(plan_size))
        self._weights: dict[str, float] = dict(DEFAULT_WEIGHTS)
        self._lock = threading.RLock()

    def _side_effect_for(self, technique: str) -> str:
        category = TECHNIQUES.get(technique, ("recon", ""))[0]
        if category == "data" or category == "logic":
            return "analyze"
        if category == "injection":
            return "analyze"
        if category == "auth":
            return "read"
        return "read"

    def _eligible(self, state: PlannerState) -> list[str]:
        return [
            technique
            for technique in TECHNIQUES
            if technique not in state.completed_techniques
            and technique not in state.blocked_techniques
        ]

    def plan(self, state: PlannerState) -> list[PlanStep]:
        """Build the next plan: highest-weight, policy-allowed techniques."""
        eligible = self._eligible(state)
        with self._lock:
            scored = []
            for technique in eligible:
                category = TECHNIQUES[technique][0]
                weight = self._weights.get(category, 0.5)
                # Penalize techniques already tried-and-failed this round.
                if technique in state.rewards and state.rewards[technique] < 0:
                    weight *= 0.4
                side_effect = self._side_effect_for(technique)
                try:
                    self.policy.assert_effect(side_effect, state.target)
                except Exception:  # noqa: BLE001 - policy block is a skip
                    continue
                scored.append((weight, technique, category, side_effect))
        scored.sort(key=lambda item: (-item[0], item[1]))
        result: list[PlanStep] = []
        for weight, technique, category, side_effect in scored[: self.plan_size]:
            result.append(
                PlanStep(
                    technique=technique,
                    category=category,
                    description=TECHNIQUES[technique][1],
                    side_effect=side_effect,
                    weight=weight,
                )
            )
        return result

    def apply_result(self, state: PlannerState, result: StepResult) -> PlannerState:
        """Update weights from a completed step and record it on the state."""
        category = TECHNIQUES.get(result.technique, ("recon", ""))[0]
        reward = max(-1.0, min(1.0, float(result.reward)))
        with self._lock:
            current = self._weights.get(category, 0.5)
            # Softmax-ish update: positive reward nudges up, negative nudges down,
            # bounded to [0.1, 2.0] so no single category dominates forever.
            updated = current + self.learning_rate * reward
            updated = max(0.1, min(2.0, updated))
            self._weights[category] = updated
        state.rewards[result.technique] = reward
        if result.status == "blocked":
            state.blocked_techniques.append(result.technique)
        else:
            state.completed_techniques.append(result.technique)
        state.round += 1
        return state

    def weights(self) -> dict[str, float]:
        with self._lock:
            return dict(self._weights)

    def reset(self) -> None:
        with self._lock:
            self._weights = dict(DEFAULT_WEIGHTS)


def plan_to_dict(plan: list[PlanStep]) -> list[dict[str, object]]:
    return [
        {
            "technique": step.technique,
            "category": step.category,
            "description": step.description,
            "side_effect": step.side_effect,
            "weight": round(step.weight, 3),
        }
        for step in plan
    ]
