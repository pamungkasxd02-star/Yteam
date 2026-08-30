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
    checks.append(check("native-tui", (ROOT / "scripts" / "yteam_tui.py").exists(), "scripts/yteam_tui.py"))
    checks.append(check("native-runtime", (ROOT / "scripts" / "yteam_runtime.py").exists(), "scripts/yteam_runtime.py"))
    checks.append(check("native-tools", (ROOT / "scripts" / "yteam_native_tools.py").exists(), "scripts/yteam_native_tools.py"))
    checks.append(check("control-plane", (ROOT / "scripts" / "yteam_control.py").exists(), "signed Telegram/Discord/WhatsApp adapter"))
    checks.append(check("durable-worker", (ROOT / "scripts" / "yteam_worker.py").exists(), "checkpointed assessment worker"))
    uv = shutil.which("uv")
    checks.append(check("uv", bool(uv), uv or "not found; installer can bootstrap it", required=False))
    camoufox = importlib.util.find_spec("camoufox") is not None
    checks.append(check("camoufox", camoufox, "installed" if camoufox else "not installed; native Botterdop remains available", required=False))
    checks.append(check("config", (ROOT / "YTEAM_SECURITY.md").exists(), "YTEAM_SECURITY.md"))
    checks.append(check("github-ci", (ROOT / ".github" / "workflows" / "ci.yml").exists(), ".github/workflows/ci.yml"))
    model_config = next((path for path in (ROOT / "yteam.local.yaml", ROOT / "runtime" / "yteam-model.yaml", ROOT / "runtime" / "yteam-model.local.yaml") if path.exists()), None)
    checks.append(check("model-config", True, str(model_config.relative_to(ROOT)) if model_config else "automatic Zen Free default (keyless)", required=False))
    usage = shutil.disk_usage(ROOT.anchor or ROOT)
    checks.append(check("disk", usage.free >= 1_000_000_000, f"{usage.free // (1024 ** 3)} GiB free", required=False))
    required_failed = [item["name"] for item in checks if item["required"] and not item["ok"]]
    return {"schema_version": 2, "product": "YTEAM", "root": str(ROOT), "ready": not required_failed, "required_failures": required_failed, "checks": checks, "next": "Install requirements.txt when optional PyYAML or Camoufox features are needed; the native runtime has no vendored-source requirement."}


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
