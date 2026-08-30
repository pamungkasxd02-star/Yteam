#!/usr/bin/env python3
"""Discover tools available to the portable Yteam security workbench."""

from __future__ import annotations

import argparse
import json
import os
import shutil
from dataclasses import asdict, dataclass
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


@dataclass(frozen=True)
class Tool:
    name: str
    purpose: str
    category: str
    command: str | None
    available: bool
    source: str
    safe_default: str


DEFINITIONS = (
    ("localsolver", "LocalSolver async Camoufox observation service", "browser", "allowlisted target; observation/manual review only"),
    ("subfinder", "Passive subdomain discovery", "recon", "passive-only"),
    ("dnsx", "DNS record and address inventory", "recon", "resolve discovered names only after scope review"),
    ("httpx", "ProjectDiscovery HTTP status/title/technology probing", "probe", "low-rate GET/HEAD"),
    ("katana", "Bounded crawler and JavaScript miner", "crawl", "bounded depth and low rate"),
    ("gau", "Passive URL collection from public archives", "recon", "passive archive queries only"),
    ("waybackurls", "Passive Wayback URL collection", "recon", "passive archive queries only"),
    ("arjun", "Controlled HTTP parameter discovery", "fuzz", "candidate endpoint only; low rate"),
    ("feroxbuster", "Bounded content discovery", "fuzz", "candidate paths only; low rate"),
    ("ffuf", "Controlled content and parameter discovery", "fuzz", "candidate paths only; rate-limited"),
    ("nuclei", "Template-based verification", "verify", "verification tags only; no DoS templates"),
    ("nmap", "Explicit service identification", "network", "operator-approved hosts/ports only"),
    ("naabu", "Explicit port discovery", "network", "operator-approved hosts only; low rate"),
    ("wafw00f", "WAF fingerprinting", "probe", "fingerprint only; no bypass spray"),
    ("sqlmap", "SQL injection verification", "verify", "candidate endpoint only"),
    ("dalfox", "XSS candidate analysis", "verify", "candidate URLs only"),
    ("smart-pipe", "Raw stream filtering and archiving", "local", "preserve raw output on D:"),
    ("search-knowledge", "Native YTEAM knowledge search", "local", "targeted queries only"),
    ("secret-scan", "Native redacted secret detection", "local", "never print secret values"),
    ("aggregate-reports", "Native target-scoped report aggregation", "local", "read/report generation only"),
)


def locate(name: str) -> tuple[str | None, str]:
    if name == "localsolver":
        script = ROOT / "scripts" / "localsolver.py"
        return (str(script), "YTEAM native") if script.exists() else (None, "missing")
    suffix = ".exe" if os.name == "nt" else ""
    for path in (
        ROOT / "runtime" / "bin" / f"{name}{suffix}",
    ):
        if path.exists() and path.is_file():
            return str(path), "project-local"
    found = shutil.which(name)
    if name == "httpx" and found and "python" in found.lower():
        return None, "missing-projectdiscovery-httpx"
    return (found, "PATH") if found else (None, "missing")


def inventory() -> list[Tool]:
    tools: list[Tool] = []
    source_commands = {"smart-pipe", "search-knowledge", "secret-scan", "aggregate-reports"}
    for name, purpose, category, safe_default in DEFINITIONS:
        command, source = locate(name)
        if command is None and name in source_commands:
            command, source = f"python scripts/yteam_native_tools.py {name}", "YTEAM native"
        tools.append(Tool(name, purpose, category, command, command is not None, source, safe_default))
    return tools


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    tools = inventory()
    if args.json:
        print(json.dumps([asdict(tool) for tool in tools], indent=2))
    else:
        for tool in tools:
            print(f"{'READY' if tool.available else 'MISSING':7} {tool.name:20} {tool.category:8} {tool.purpose} [{tool.source}]")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
