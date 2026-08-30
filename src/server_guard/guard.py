"""Read-only server hardening / exposure checks for authorized assessment."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class HeaderCheck:
    name: str
    description: str
    expected: str | None = None


HEADER_CHECKS: list[HeaderCheck] = [
    HeaderCheck("content-security-policy", "Restrict script/style sources"),
    HeaderCheck("x-frame-options", "Clickjacking protection", "SAMEORIGIN|DENY"),
    HeaderCheck("strict-transport-security", "Enforce HTTPS"),
    HeaderCheck("x-content-type-options", "Prevent MIME sniffing", "nosniff"),
    HeaderCheck("referrer-policy", "Limit referrer leakage"),
    HeaderCheck("permissions-policy", "Restrict browser features"),
]


def check_headers(headers: dict[str, str] | None) -> list[dict]:
    """Evaluate security headers; returns pass/warn/na per header (read-only)."""
    headers = {str(k).lower(): str(v) for k, v in (headers or {}).items()}
    results: list[dict] = []
    for check in HEADER_CHECKS:
        value = headers.get(check.name)
        if value is None:
            results.append({"name": check.name, "status": "missing", "found": None})
            continue
        results.append({"name": check.name, "status": "present", "found": value})
    return results


def build_guard_report(headers: dict[str, str] | None, exposed_paths: list[str] | None = None) -> dict:
    header_results = check_headers(headers)
    exposed = [path for path in (exposed_paths or []) if path]
    return {
        "headers": header_results,
        "missing_headers": [item["name"] for item in header_results if item["status"] == "missing"],
        "exposed_paths": exposed,
        "recommendations": [
            f"Set {name}" for name in [item["name"] for item in header_results if item["status"] == "missing"]
        ] + [f"Review exposure: {path}" for path in exposed],
        "non_claim": "Guard report is a hardening/monitoring signal for your own or authorized servers; it is not a vulnerability verdict.",
    }
