"""Detect encoded / encrypted / signed payload formats for authorized analysis."""

from __future__ import annotations

import base64
import binascii
import hashlib
import json
import re


HEX_RE = re.compile(r"^[0-9a-fA-F]{8,}$")
BASE64_RE = re.compile(r"^[A-Za-z0-9+/]+={0,2}$")
JWT_RE = re.compile(r"^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$")


def _try_base64(text: str) -> bytes | None:
    try:
        decoded = base64.b64decode(text, validate=True)
        if decoded:
            return decoded
    except (binascii.Error, ValueError):
        return None
    return None


def _looks_json(blob: bytes) -> bool:
    try:
        json.loads(blob.decode("utf-8", errors="replace"))
        return True
    except (ValueError, UnicodeDecodeError):
        return False


def detect_encoding(text: str) -> list[str]:
    """Return detected encodings in priority order."""
    detected: list[str] = []
    stripped = text.strip()
    if not stripped:
        return detected
    if HEX_RE.match(stripped) and len(stripped) % 2 == 0:
        detected.append("hex")
    decoded_base64 = _try_base64(stripped) if len(stripped) % 4 == 0 else None
    if BASE64_RE.match(stripped) and decoded_base64 is not None and "hex" not in detected:
        detected.append("base64")
    if JWT_RE.match(stripped):
        detected.append("jwt")
    if stripped.isdigit():
        detected.append("numeric")
    if re.fullmatch(r"[A-Za-z0-9+/=]{8,}", stripped) and "base64" not in detected:
        detected.append("opaque-b64-like")
    return detected


def analyze_payload(text: str) -> dict:
    """Analyze a payload and report what it might be (without decoding secrets)."""
    encodings = detect_encoding(text)
    result: dict = {
        "length": len(text),
        "encodings": encodings,
        "entropy_guess": None,
        "decoded_preview": None,
        "looks_like": [],
        "note": "Analysis is heuristic; treat results as guidance for authorized testing.",
    }
    if "base64" in encodings:
        decoded = _try_base64(text)
        if decoded is not None:
            result["decoded_preview"] = decoded[:200].decode("utf-8", errors="replace")
            if _looks_json(decoded):
                result["looks_like"].append("json-in-base64")
            else:
                result["looks_like"].append("base64-blob")
    if "jwt" in encodings:
        parts = text.split(".")
        if len(parts) == 3:
            header = _try_base64(parts[0] + "=" * (-len(parts[0]) % 4))
            if header:
                try:
                    header_json = json.loads(header)
                    result["looks_like"].append(f"jwt-header:{header_json.get('alg')}")
                except ValueError:
                    result["looks_like"].append("jwt-header:unparsed")
    if "hex" in encodings:
        result["looks_like"].append("hex-encoded")
    return result
