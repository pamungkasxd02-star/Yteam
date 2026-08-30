#!/usr/bin/env python3
"""Diagnose Yteam installation without changing the host or network state."""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
import shutil
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def check(name: str, ok: bool, detail: str, required: bool = True) -> dict[str, object]:
    return {"name": name, "ok": ok, "required": required, "detail": detail}


def run() -> dict[str, object]:
    checks: list[dict[str, object]] = []
    checks.append(check("python", sys.version_info >= (3, 11) and sys.version_info < (3, 14), sys.version.split()[0]))
    checks.append(check("opencode-source", (ROOT / "vendor" / "opencode" / "packages" / "opencode" / "src" / "index.ts").exists(), "vendor/opencode/packages/opencode/src/index.ts"))
    checks.append(check("hermes-source", (ROOT / "vendor" / "hermes-agent" / "hermes").exists(), "vendor/hermes-agent/hermes"))
    checks.append(check("cybermes-source", (ROOT / "vendor" / "cybermes" / "go.mod").exists(), "vendor/cybermes/go.mod"))
    bun = shutil.which("bun") or os.environ.get("BUN_BIN", "")
    checks.append(check("bun", bool(bun), bun or "not found"))
    go = shutil.which("go")
    checks.append(check("go", bool(go), go or "not found", required=False))
    uv = shutil.which("uv")
    checks.append(check("uv", bool(uv), uv or "not found"))
    camoufox = importlib.util.find_spec("camoufox") is not None
    checks.append(check("camoufox", camoufox, "installed" if camoufox else "not installed; native Botterdop remains available", required=False))
    checks.append(check("config", (ROOT / "opencode.json").exists() and (ROOT / "YTEAM_SECURITY.md").exists(), "opencode.json + YTEAM_SECURITY.md"))
    checks.append(check("tui-overlay", (ROOT / ".opencode" / "plugins" / "yteam-tui.tsx").exists(), ".opencode/plugins/yteam-tui.tsx"))
    checks.append(check("github-ci", (ROOT / ".github" / "workflows" / "ci.yml").exists(), ".github/workflows/ci.yml"))
    model_config = next((path for path in (ROOT / "yteam.local.yaml", ROOT / "runtime" / "yteam-model.yaml", ROOT / "runtime" / "yteam-model.local.yaml") if path.exists()), None)
    checks.append(check("model-config", True, str(model_config.relative_to(ROOT)) if model_config else "automatic OpenCode Zen Free default (keyless)", required=False))
    usage = shutil.disk_usage(ROOT.anchor or ROOT)
    checks.append(check("disk", usage.free >= 1_000_000_000, f"{usage.free // (1024 ** 3)} GiB free", required=False))
    required_failed = [item["name"] for item in checks if item["required"] and not item["ok"]]
    return {"schema_version": 1, "product": "YTEAM", "root": str(ROOT), "ready": not required_failed, "required_failures": required_failed, "checks": checks, "next": "Install missing required dependencies; Camoufox and Go are optional."}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    result = run()
    if args.json:
        print(json.dumps(result, indent=2))
    else:
        print(f"YTEAM doctor: {'READY' if result['ready'] else 'BLOCKED'}")
        for item in result["checks"]:
            print(f"{'OK' if item['ok'] else 'MISSING':7} {item['name']:18} {item['detail']}")
        if result["required_failures"]:
            print("Required fixes: " + ", ".join(str(item) for item in result["required_failures"]))
    return 0 if result["ready"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
