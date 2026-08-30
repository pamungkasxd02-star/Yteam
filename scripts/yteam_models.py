#!/usr/bin/env python3
"""Native YTEAM model configuration and Zen Free catalog discovery."""

from __future__ import annotations

import json
import urllib.error
import urllib.request
from pathlib import Path

try:
    import yaml
except ImportError:  # pragma: no cover - installer declares PyYAML
    yaml = None


DEFAULT_MODEL_CONFIG: dict[str, str] = {
    "provider": "zen-free",
    "model": "laguna-s-2.1-free",
    "api_key": "",
    "base_url": "https://opencode.ai/zen/v1",
}
FREE_MODELS_URL = "https://opencode.ai/zen/v1/models"
FREE_MODEL_FALLBACK = (
    "big-pickle",
    "deepseek-v4-flash-free",
    "muse-spark-1.2-contributor-free",
    "mimo-v2.5-free",
    "ling-3.0-flash-fin-free",
    "nemotron-3-ultra-free",
    "nemotron-3.5-lightning-free",
    "laguna-s-2.1-free",
)


def load_model_config(root: Path) -> dict[str, str]:
    """Load an optional local override, defaulting to keyless Zen Free."""
    candidates = (
        root / "yteam.local.yaml",
        root / "runtime" / "yteam-model.yaml",
        root / "runtime" / "yteam-model.local.yaml",
    )
    selected = next((path for path in candidates if path.exists()), None)
    if selected is None:
        return dict(DEFAULT_MODEL_CONFIG)
    if yaml is None:
        raise RuntimeError("PyYAML is required to read yteam.local.yaml")
    try:
        data = yaml.safe_load(selected.read_text(encoding="utf-8")) or {}
    except (OSError, ValueError, yaml.YAMLError) as error:
        raise RuntimeError(f"Invalid model config {selected}: {error}") from error
    if not isinstance(data, dict):
        raise RuntimeError("yteam.local.yaml must contain a YAML object")
    result = dict(DEFAULT_MODEL_CONFIG)
    result.update({key: str(data[key]).strip() for key in ("provider", "model", "api_key", "base_url") if key in data})
    if not result.get("provider") or not result.get("model"):
        raise RuntimeError("yteam.local.yaml requires non-empty provider and model fields")
    if not result.get("api_key") and result["provider"].lower() not in {"zen-free", "zen_free"}:
        raise RuntimeError("A non-free provider requires api_key in yteam.local.yaml")
    return result


def discover_free_models(timeout: float = 7.0) -> list[str]:
    request = urllib.request.Request(FREE_MODELS_URL, headers={"Accept": "application/json", "User-Agent": "YTEAM/1.0"})
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            payload = json.loads(response.read().decode("utf-8"))
        entries = payload.get("data", []) if isinstance(payload, dict) else []
        models = sorted({
            str(item.get("id", "")).strip()
            for item in entries
            if isinstance(item, dict) and (str(item.get("id", "")).endswith("-free") or str(item.get("id", "")) == "big-pickle")
        })
        if models:
            return models
    except (OSError, ValueError, TypeError, urllib.error.URLError):
        pass
    return list(FREE_MODEL_FALLBACK)
