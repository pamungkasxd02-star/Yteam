#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PYTHON="${YTEAM_PYTHON:-python3}"
if ! command -v "$PYTHON" >/dev/null 2>&1; then PYTHON=python; fi
exec "$PYTHON" "$SCRIPT_DIR/scripts/yteam_uninstall.py" "$@"
