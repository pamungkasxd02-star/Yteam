#!/usr/bin/env python3
"""Run the optional isolated Camoufox Botterdop observation pass."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))

from bot_bypass.camoufox_adapter import CamoufoxConfig, run_camoufox
from yteam_scope import validate


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target", help="Authorized HTTP(S) target")
    parser.add_argument("--output", type=Path, default=ROOT / "runtime" / "botterdop")
    parser.add_argument("--scope-file", type=Path, help="Explicit YAML scope file; without it only the exact target is allowed")
    parser.add_argument("--headed", action="store_true", help="Show the isolated browser for manual authorized review")
    parser.add_argument("--rate", type=float, default=1.0)
    args = parser.parse_args()

    decision = validate(args.target, "", args.scope_file)
    output = args.output.resolve()
    if not decision.allowed:
        result = {"status": "blocked", "engine": "camoufox", "target": args.target, "scope": decision.__dict__, "action": "stop"}
        output.mkdir(parents=True, exist_ok=True)
        (output / "camoufox.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(result, indent=2))
        return 2

    result = run_camoufox(CamoufoxConfig(args.target, output, headless=not args.headed, rate=args.rate))
    print(json.dumps(result, indent=2, default=str))
    return 0 if result.get("status") in {"completed", "unavailable"} else 2


if __name__ == "__main__":
    raise SystemExit(main())
