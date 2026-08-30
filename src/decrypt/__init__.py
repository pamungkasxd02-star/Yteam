"""Decrypt / reverse-engineering pillar for authorized testing and research.

Purpose: recognize and analyze encrypted, encoded, or signed response formats
and client-side crypto mechanisms so you can test and understand YOUR OWN or
authorized applications. This is not for defeating encryption of systems you
do not own or are not authorized to assess.
"""

from .detect import detect_encoding, analyze_payload

__all__ = ["detect_encoding", "analyze_payload"]
