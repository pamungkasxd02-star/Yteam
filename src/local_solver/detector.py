"""LocalSolver gate classification and safe request governance.

LocalSolver observes anti-automation responses and decides whether an
authorized recon run should continue, slow down, or stop for manual review. It
does not evade controls, rotate identities, solve CAPTCHAs, or spray requests.
"""

from __future__ import annotations

import re
import time
from dataclasses import dataclass
from enum import Enum


class GateKind(str, Enum):
    CLOUDFLARE_CHALLENGE = "cloudflare_challenge"
    CLOUDFLARE_MANAGED = "cloudflare_managed"
    AKAMAI_KPSDK = "akamai_kpsdk"
    DATADOME = "datadome"
    KASADA = "kasada"
    PERIMETERX_HUMAN = "perimeterx_human"
    AWS_WAF = "aws_waf"
    FASTLY_WAF = "fastly_waf"
    GENERIC_WAF = "generic_waf"
    RATE_LIMIT = "rate_limit"
    RECAPTCHA_V2 = "recaptcha_v2"
    RECAPTCHA_ENTERPRISE = "recaptcha_enterprise"
    TURNSTILE = "turnstile"
    CUSTOM_403 = "custom_403"
    NONE = "none"


GATE_MARKERS: dict[GateKind, tuple[str, ...]] = {
    GateKind.CLOUDFLARE_CHALLENGE: ("cf-ray", "cf-chl", "just a moment", "managed_check", "challenge-platform"),
    GateKind.CLOUDFLARE_MANAGED: ("cf-mitigated", "__cf_bm", "cf_chl_opt"),
    GateKind.AKAMAI_KPSDK: ("window.KPSDK", "akamai-bm", "_abck", "bm-verify"),
    GateKind.DATADOME: ("datadome", "dd_cookie_test", "datadome-captcha"),
    GateKind.KASADA: ("kasada", "x-kpsdk-cid", "x-kpsdk-ct"),
    GateKind.PERIMETERX_HUMAN: ("perimeterx", "human security", "px-captcha", "_px3", "_pxvid"),
    GateKind.AWS_WAF: ("awswaf", "aws-waf-token", "x-amzn-waf-action"),
    GateKind.FASTLY_WAF: ("fastly-waf", "fastly error", "x-fastly-request-id"),
    GateKind.GENERIC_WAF: ("imperva", "incap_ses", "visid_incap", "sucuri", "x-sucuri-id", "f5 big-ip", "bigipserver"),
    GateKind.RECAPTCHA_V2: ("grecaptcha.render", "recaptcha/api.js", "recaptcha__en"),
    GateKind.RECAPTCHA_ENTERPRISE: ("recaptchaenterprise", "enterprise.recaptcha", "recaptcha/enterprise"),
    GateKind.TURNSTILE: ("cf-turnstile", "turnstile/api.js", "challenges.cloudflare.com"),
}


@dataclass(frozen=True)
class LocalSolverDecision:
    gate: str
    category: str
    confidence: float
    evidence: tuple[str, ...]
    action: str
    retry_after_seconds: float
    status: int | None
    note: str


class LocalSolver:
    """Stateful detector/governor that fails closed at anti-bot boundaries."""

    def __init__(self, base_rate: float = 1.0) -> None:
        self.base_rate = max(0.1, min(float(base_rate), 10.0))
        self.current_delay = 1.0 / self.base_rate
        self.last_request = 0.0
        self.blocked = False
        self.halted = False
        self.last_decision: LocalSolverDecision | None = None

    def wait(self) -> None:
        delay = self.current_delay - (time.monotonic() - self.last_request)
        if delay > 0:
            time.sleep(delay)
        self.last_request = time.monotonic()

    def inspect(self, headers: dict[str, str] | None, body: str = "", status: int | None = None) -> LocalSolverDecision:
        headers = {str(key).lower(): str(value) for key, value in (headers or {}).items()}
        combined = " ".join(f"{key}:{value}" for key, value in headers.items()).lower() + " " + body.lower()
        evidence: list[str] = []
        kind = GateKind.NONE
        for candidate, markers in GATE_MARKERS.items():
            matched = [marker for marker in markers if marker.lower() in combined]
            if matched:
                passive = {"cf-ray", "_abck", "_px3", "_pxvid", "x-fastly-request-id"}
                if candidate in {GateKind.CLOUDFLARE_CHALLENGE, GateKind.CLOUDFLARE_MANAGED, GateKind.AKAMAI_KPSDK, GateKind.PERIMETERX_HUMAN, GateKind.FASTLY_WAF} and all(marker.lower() in passive or marker.lower() == "__cf_bm" for marker in matched) and status not in {401, 403, 429, 503}:
                    continue
                kind = candidate
                evidence.extend(matched)
                break
        if kind == GateKind.NONE and status == 429:
            kind = GateKind.RATE_LIMIT
            evidence.append("HTTP 429")
        if kind == GateKind.NONE and status in {401, 403} and any(marker in combined for marker in ("access denied", "request blocked", "forbidden", "security policy", "bot detected")):
            kind = GateKind.GENERIC_WAF
            evidence.append("blocked response wording")
        if kind == GateKind.NONE:
            decision = LocalSolverDecision("none", "none", 0.0, (), "continue", 0.0, status, "No known anti-automation gate marker detected.")
            self.last_decision = decision
            return decision
        if kind in {GateKind.RECAPTCHA_V2, GateKind.RECAPTCHA_ENTERPRISE, GateKind.TURNSTILE}:
            category, action, retry = "captcha", "manual_review", 0.0
        elif kind == GateKind.RATE_LIMIT:
            try:
                retry_after = float(headers.get("retry-after", "0") or 0)
            except ValueError:
                retry_after = 0.0
            category, action, retry = "rate_limit", "slow_down", max(2.0, retry_after)
        elif kind in {GateKind.CLOUDFLARE_CHALLENGE, GateKind.CLOUDFLARE_MANAGED, GateKind.AKAMAI_KPSDK, GateKind.DATADOME, GateKind.KASADA, GateKind.PERIMETERX_HUMAN}:
            category, action, retry = "bot_gate", "stop", 0.0
        else:
            category, action, retry = "waf", "stop", 0.0
        if action == "slow_down":
            self.current_delay = max(self.current_delay * 2.0, retry)
        if action in {"stop", "manual_review"}:
            self.halted = True
        if action == "stop":
            self.blocked = True
        confidence = min(1.0, 0.65 + min(len(evidence), 3) * 0.1 + (0.1 if status in {403, 429, 503} else 0.0))
        decision = LocalSolverDecision(kind.value, category, confidence, tuple(sorted(set(evidence))), action, retry, status, "Observation only; no bypass, evasion, or challenge solving attempted.")
        self.last_decision = decision
        return decision

    def summary(self) -> dict[str, object]:
        return {
            "blocked": self.blocked,
            "halted": self.halted,
            "base_rate_rps": self.base_rate,
            "current_delay_seconds": round(self.current_delay, 3),
            "last_decision": self.last_decision.__dict__ if self.last_decision else None,
            "policy": "observe, classify, rate-limit, and stop safely; never evade",
        }


def classify_response(headers: dict[str, str] | None, body: str = "", status: int | None = None) -> GateKind:
    decision = LocalSolver().inspect(headers, body, status)
    if decision.gate != GateKind.NONE.value:
        return GateKind(decision.gate)
    if status == 403 and body.strip() and not body.lower().startswith("<!doctype"):
        return GateKind.CUSTOM_403
    return GateKind.NONE


def gate_summary(headers: dict[str, str] | None, body: str = "", status: int | None = None) -> dict[str, object]:
    decision = LocalSolver().inspect(headers, body, status)
    return {"gate": decision.gate, "category": decision.category, "known_gate": decision.gate != GateKind.NONE.value, "confidence": decision.confidence, "evidence": list(decision.evidence), "action": decision.action, "retry_after_seconds": decision.retry_after_seconds, "status": decision.status, "note": decision.note}
