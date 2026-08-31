"""Knowledge graph + verified lesson ledger for YTEAM.

Unlike the flat JSONL memory, the knowledge graph stores typed nodes (targets,
findings, techniques, lessons) and labeled edges (observed_on, blocked_by,
verified_by, refines, contradicts). It supports traversal queries (what did we
learn across targets for this technique family?) and aggregates verified
lessons into model context.

The graph is append-only and redacted at write time. Reads are filtered by an
optional query that matches node labels, node kinds, and edge kinds.
"""

from __future__ import annotations

import json
import re
import secrets
from pathlib import Path
from typing import Iterable

NODE_KINDS = {"target", "finding", "technique", "lesson", "blocker"}
EDGE_KINDS = {"observed_on", "blocked_by", "verified_by", "refines", "contradicts", "supports"}


class KnowledgeError(ValueError):
    """Raised on invalid graph writes."""


def _clean_text(value: str, limit: int = 1200) -> str:
    # Redact common secret shapes without depending on the runtime safety module.
    value = re.sub(r"(?i)(bearer\s+|session=|token=|password\s*[:=]|api[_-]?key\s*[:=])[^\s,;]+", r"\1<REDACTED>", value)
    return value.strip()[:limit]


class KnowledgeGraph:
    """Append-only knowledge graph persisted as JSONL."""

    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)

    # -- persistence ------------------------------------------------------
    def _records(self) -> list[dict[str, object]]:
        if not self.path.exists():
            return []
        records: list[dict[str, object]] = []
        for line in self.path.read_text(encoding="utf-8", errors="replace").splitlines():
            try:
                value = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(value, dict):
                records.append(value)
        return records

    def _append(self, record: dict[str, object]) -> dict[str, object]:
        with self.path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(record, ensure_ascii=False, sort_keys=True) + "\n")
        return record

    # -- writes -----------------------------------------------------------
    def add_node(self, kind: str, label: str, props: dict[str, object] | None = None) -> dict[str, object]:
        if kind not in NODE_KINDS:
            raise KnowledgeError(f"unknown node kind: {kind}")
        node_id = "kn_" + secrets.token_hex(8)
        return self._append({
            "type": "node",
            "id": node_id,
            "kind": kind,
            "label": _clean_text(label, 160),
            "props": _clean_props(props or {}),
            "created_at": _now(),
        })

    def add_edge(self, source_id: str, kind: str, target_id: str, props: dict[str, object] | None = None) -> dict[str, object]:
        if kind not in EDGE_KINDS:
            raise KnowledgeError(f"unknown edge kind: {kind}")
        edge_id = "ke_" + secrets.token_hex(8)
        return self._append({
            "type": "edge",
            "id": edge_id,
            "kind": kind,
            "source": source_id,
            "target": target_id,
            "props": _clean_props(props or {}),
            "created_at": _now(),
        })

    def lesson(self, text: str, tags: Iterable[str] = (), verified_by: str = "operator") -> dict[str, object]:
        node = self.add_node("lesson", text, {"tags": sorted({str(tag)[:40] for tag in tags}), "verified_by": verified_by, "status": "verified"})
        return node

    # -- queries ----------------------------------------------------------
    def nodes(self, kind: str | None = None, query: str = "", limit: int = 50) -> list[dict[str, object]]:
        records = [item for item in self._records() if item.get("type") == "node"]
        if kind:
            records = [item for item in records if item.get("kind") == kind]
        if query:
            words = [word for word in re.findall(r"[a-z0-9_-]+", query.lower()) if len(word) > 2]
            records = [item for item in records if _matches_words(item, words)]
        return records[-max(1, min(limit, 200)):]

    def edges(self, kind: str | None = None, node_id: str | None = None, limit: int = 100) -> list[dict[str, object]]:
        records = [item for item in self._records() if item.get("type") == "edge"]
        if kind:
            records = [item for item in records if item.get("kind") == kind]
        if node_id:
            records = [item for item in records if item.get("source") == node_id or item.get("target") == node_id]
        return records[-max(1, min(limit, 400)):]

    def neighbors(self, node_id: str, max_depth: int = 2, limit: int = 50) -> list[dict[str, object]]:
        """Breadth-first traversal from a node, returning connected records."""
        seen: set[str] = set()
        frontier = [node_id]
        found: list[dict[str, object]] = []
        for _ in range(max_depth):
            if not frontier:
                break
            next_frontier: list[str] = []
            for current in frontier:
                for edge in self.edges(node_id=current):
                    other = edge["target"] if edge["source"] == current else edge["source"]
                    found.append(edge)
                    if other not in seen:
                        seen.add(other)
                        node = self._by_id(other)
                        if node:
                            found.append(node)
                            next_frontier.append(other)
            frontier = next_frontier
            if len(found) >= limit:
                break
        return found[:limit]

    def learned_context(self, query: str = "", limit: int = 8) -> str:
        lessons = self.nodes(kind="lesson", query=query, limit=limit)
        if not lessons:
            return "No verified knowledge-graph lessons are available for this request."
        return "\n".join(f"- {item.get('label', '')}" for item in lessons)

    def _by_id(self, node_id: str) -> dict[str, object] | None:
        for item in self._records():
            if item.get("type") == "node" and item.get("id") == node_id:
                return item
        return None

    def summary(self) -> dict[str, int]:
        records = self._records()
        kinds: dict[str, int] = {}
        for item in records:
            if item.get("type") == "node":
                kinds[str(item.get("kind"))] = kinds.get(str(item.get("kind")), 0) + 1
        return {"records": len(records), "nodes": sum(kinds.values()), "kinds": kinds, "edges": sum(1 for item in records if item.get("type") == "edge")}


def _clean_props(props: dict[str, object]) -> dict[str, object]:
    return {str(key)[:60]: _clean_value(value) for key, value in props.items()}


def _clean_value(value: object) -> object:
    if isinstance(value, str):
        return _clean_text(value, 400)
    if isinstance(value, (list, tuple)):
        return [_clean_value(item) for item in value][:100]
    if isinstance(value, dict):
        return _clean_props(value)
    return value


def _matches_words(record: dict[str, object], words: list[str]) -> bool:
    haystack = " ".join([
        str(record.get("kind", "")),
        str(record.get("label", "")),
        json.dumps(record.get("props", {}), sort_keys=True),
    ]).lower()
    return all(word in haystack for word in words)


def _now() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).isoformat()
