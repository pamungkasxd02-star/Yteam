#!/usr/bin/env python3
"""Fetch Yteam's upstream source dependencies into vendor/.

The default ``runtime`` profile checks out only source needed to run YTEAM.
The ``full`` profile is for CI and contributors who need upstream tests, docs,
and development tooling.
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
VENDOR = ROOT / "vendor"
SOURCES = {
    "opencode": ("https://github.com/anomalyco/opencode.git", "dev"),
    "hermes-agent": ("https://github.com/NousResearch/hermes-agent.git", "main"),
    "cybermes": ("https://github.com/Zyrexnn/Cybermes.git", "main"),
}
CYBERMES_SPARSE = [
    ".hermes", "cmd", "pkg", "skills", "scripts", "tools",
    "README.md", "AGENTS.md", "ATTRIBUTION.md", "LICENSE", "go.mod", "go.sum",
    "package.json", "pyproject.toml", "requirements.txt", "scope.yaml",
    "!tools/bin/**", "!tools/sqlmap/**",
    # These small text-only slices power targeted Cybermes knowledge lookup;
    # the multi-source corpus is intentionally not downloaded in runtime mode.
    "knowledge/PayloadsAllTheThings/JSON Web Token",
    "knowledge/hack-skills/skills/jwt-oauth-token-attacks",
    "knowledge/Claude-BugHunter/skills/hunt-jwt-crypto",
    "knowledge/strix-skills/vulnerabilities/authentication_jwt.md",
]

HERMES_RUNTIME_SPARSE = [
    "agent", "gateway", "hermes_cli", "cron", "acp_adapter", "plugins",
    "providers", "skills", "tools", "tui_gateway", "run_agent.py",
    "registration_lifecycle.py", "model_tools.py", "toolsets.py",
    "batch_runner.py", "trajectory_compressor.py", "toolset_distributions.py",
    "cli.py", "hermes_bootstrap.py", "hermes_constants.py", "hermes_state.py",
    "hermes_state_common.py", "hermes_state_portability.py", "hermes_state_schema.py",
    "hermes_state_search.py", "hermes_time.py", "hermes_logging.py", "utils.py",
    "mcp_serve.py", "pyproject.toml", "uv.lock", "setup.py", "hermes",
    "README.md", "LICENSE",
]

OPENCODE_RUNTIME_SPARSE = [
    "package.json", "bun.lock", "bunfig.toml", "tsconfig.json", "turbo.json",
    "patches", "packages/opencode", "packages/core", "packages/llm",
    "packages/protocol", "packages/schema", "packages/server", "packages/tui",
    "packages/ui", "packages/plugin", "packages/sdk/js", "packages/script",
    "packages/codemode", "packages/effect-drizzle-sqlite", "packages/effect-sqlite-node",
    "packages/http-recorder",
]
DIRECTORY_PATTERNS = {
    "agent", "gateway", "hermes_cli", "cron", "acp_adapter", "plugins",
    "providers", "skills", "tools", "tui_gateway", "hermes", "patches",
    "packages/opencode", "packages/core", "packages/llm", "packages/protocol",
    "packages/schema", "packages/server", "packages/tui", "packages/ui",
    "packages/plugin", "packages/sdk/js", "packages/script", "packages/codemode",
    "packages/effect-drizzle-sqlite", "packages/effect-sqlite-node",
    "packages/http-recorder", "cmd", "pkg", "scripts",
}


def sparse_patterns(name: str, profile: str) -> list[str]:
    """Return checkout patterns while excluding development-only payloads."""
    if profile == "full":
        return []
    if name == "hermes-agent":
        includes = HERMES_RUNTIME_SPARSE
    elif name == "opencode":
        includes = OPENCODE_RUNTIME_SPARSE
    else:
        includes = CYBERMES_SPARSE
    excludes = [
        "!**/test/**", "!**/tests/**", "!**/__snapshots__/**",
        "!**/*.test.ts", "!**/*.test.tsx", "!**/*.spec.ts", "!**/*.spec.tsx",
        "!**/*.stories.ts", "!**/*.stories.tsx", "!**/e2e/**",
        "!**/docs/**", "!**/website/**", "!**/examples/**", "!**/evals/**",
    ]
    expanded = [f"{item}/**" if item in DIRECTORY_PATTERNS else item for item in includes]
    return [*expanded, *excludes]


def run(command: list[str], cwd: Path | None = None) -> None:
    result = subprocess.run(command, cwd=cwd, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(command)}")


def sparse_checkout(destination: Path, name: str, profile: str) -> None:
    if profile == "full":
        run(["git", "-C", str(destination), "sparse-checkout", "disable"])
        return
    run(["git", "-C", str(destination), "sparse-checkout", "set", "--skip-checks", "--no-cone", *sparse_patterns(name, profile)])


def clone(name: str, url: str, branch: str, refresh: bool, profile: str) -> None:
    destination = VENDOR / name
    if destination.exists() and (destination / ".git").exists():
        if refresh:
            run(["git", "-C", str(destination), "fetch", "--depth", "1", "origin", branch])
            run(["git", "-C", str(destination), "checkout", branch])
            run(["git", "-C", str(destination), "reset", "--hard", f"origin/{branch}"])
        if profile == "full":
            # Explicit --full-sources expands a sparse checkout but does not
            # reset or overwrite contributor edits.
            result = subprocess.run(
                ["git", "-C", str(destination), "config", "--get", "core.sparseCheckout"],
                check=False,
                capture_output=True,
                text=True,
            )
            if result.returncode == 0 and result.stdout.strip().lower() == "true":
                sparse_checkout(destination, name, profile)
        # Do not prune an existing checkout implicitly. A contributor may have
        # a full tree or local edits; the sparse profile applies to fresh
        # installs and can be explicitly requested with --refresh.
        return
    if destination.exists():
        raise RuntimeError(f"destination exists but is not a Git checkout: {destination}")
    if name == "cybermes" and profile != "full":
        run(["git", "clone", "--filter=blob:none", "--no-checkout", "--sparse", "--branch", branch, url, str(destination)])
        run(["git", "-C", str(destination), "config", "core.longpaths", "true"])
        sparse_checkout(destination, name, profile)
        run(["git", "-C", str(destination), "checkout", branch])
        return
    if profile == "full":
        run(["git", "clone", "--depth", "1", "--branch", branch, url, str(destination)])
        return
    run(["git", "clone", "--filter=blob:none", "--no-checkout", "--sparse", "--branch", branch, url, str(destination)])
    run(["git", "-C", str(destination), "config", "core.longpaths", "true"])
    sparse_checkout(destination, name, profile)
    run(["git", "-C", str(destination), "checkout", branch])


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--refresh", action="store_true", help="fetch and reset existing vendor checkouts")
    parser.add_argument("--profile", choices=("runtime", "full"), default="runtime", help="runtime is lean; full is for CI/contributors")
    args = parser.parse_args()
    VENDOR.mkdir(parents=True, exist_ok=True)
    try:
        for name, (url, branch) in SOURCES.items():
            clone(name, url, branch, args.refresh, args.profile)
    except (OSError, RuntimeError) as error:
        print(f"bootstrap_sources: {error}", file=sys.stderr)
        return 2
    print(f"Yteam upstream sources are ready under vendor/ ({args.profile} profile).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
