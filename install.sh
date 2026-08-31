#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
BOOTSTRAP_URL="https://raw.githubusercontent.com/pamungkasxd02-star/Yteam/main/install.py"
PYTHON=""
if command -v python3 >/dev/null 2>&1; then PYTHON=python3; fi
if [ -z "$PYTHON" ] && command -v python >/dev/null 2>&1; then PYTHON=python; fi
if [ -n "$PYTHON" ] && [ -f "$SCRIPT_DIR/install.py" ]; then
  exec "$PYTHON" "$SCRIPT_DIR/install.py" "$@"
fi
if [ -n "$PYTHON" ] && command -v curl >/dev/null 2>&1; then
  exec "$PYTHON" -c "$(curl -fsSL "$BOOTSTRAP_URL")" -- "$@"
fi
if [ -n "$PYTHON" ] && command -v wget >/dev/null 2>&1; then
  exec "$PYTHON" -c "$(wget -qO- "$BOOTSTRAP_URL")" -- "$@"
fi
echo "YTEAM requires Python 3.11, 3.12, or 3.13. Install Python and run this script again." >&2
exit 2
