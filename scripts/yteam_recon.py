#!/usr/bin/env python3
"""Deep, low-rate, read-only web recon engine for the Yteam /bb pipeline."""

from __future__ import annotations

import argparse
import ipaddress
import json
import re
import socket
import ssl
import sys
import time
from collections import defaultdict
from dataclasses import asdict, dataclass, field
from html.parser import HTMLParser
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin, urlparse
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))

from local_solver.detector import LocalSolver
from yteam_safety import redact_text, redact_url, redact_value
DEFAULT_OUTPUT = ROOT / "runtime" / "recon"
ATTRIBUTION = "pamungkas"
ROUTE_HINTS = {
    "admin": 35, "internal": 40, "debug": 40, "graphql": 35, "swagger": 25,
    "openapi": 25, "api": 20, "auth": 20, "oauth": 25, "login": 20,
    "upload": 25, "export": 25, "download": 20, "users": 15, "account": 20,
    "orders": 20, "billing": 30, "invoice": 30, "webhook": 30, "callback": 25,
}
STATIC_SUFFIXES = (".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".map")
DOC_PATHS = (
    "/robots.txt", "/sitemap.xml", "/.well-known/security.txt", "/.well-known/openapi.json",
    "/openapi.json", "/swagger.json", "/swagger/v1/swagger.json", "/api-docs", "/graphql",
)
INLINE_ROUTE_RE = re.compile(r"[\"'`]((?:https?://[^\"'`\s]+)?/(?:api|v[0-9]+|rest|graphql|gql|admin|internal|auth|oauth|login|users|accounts|orders|invoices|documents|upload|download|export|webhooks?)[^\"'`\s]*)", re.IGNORECASE)


@dataclass
class HTTPObservation:
    url: str
    status: int | None
    content_type: str
    length: int
    elapsed_ms: int
    title: str = ""
    redirect: str = ""
    server: str = ""
    technologies: list[str] = field(default_factory=list)
    security_headers: dict[str, str] = field(default_factory=dict)
    localsolver: dict[str, object] = field(default_factory=dict)
    error: str = ""


class PageParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.links: set[str] = set()
        self.scripts: set[str] = set()
        self.forms: list[dict[str, str]] = []
        self.title: list[str] = []
        self._in_title = False

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        values = {key.lower(): value or "" for key, value in attrs}
        if tag.lower() in {"a", "link", "iframe", "img", "source"} and (values.get("href") or values.get("src")):
            self.links.add(values.get("href") or values.get("src"))
        if tag.lower() == "script" and values.get("src"):
            self.scripts.add(values["src"])
        if tag.lower() == "form":
            self.forms.append({"action": values.get("action", ""), "method": values.get("method", "GET").upper()})
        if tag.lower() == "title":
            self._in_title = True

    def handle_endtag(self, tag: str) -> None:
        if tag.lower() == "title":
            self._in_title = False

    def handle_data(self, data: str) -> None:
        if self._in_title:
            self.title.append(data.strip())


def slugify(target: str) -> str:
    parsed = urlparse(target if "://" in target else f"https://{target}")
    return re.sub(r"[^a-zA-Z0-9]+", "_", parsed.netloc or parsed.path).strip("_").lower()[:96] or "target"


def normalize(target: str) -> str:
    value = target.strip()
    if not value:
        raise ValueError("target is empty")
    if "://" not in value:
        value = "https://" + value
    parsed = urlparse(value)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname:
        raise ValueError("target must be an HTTP(S) URL or hostname")
    return value.rstrip("/")


def same_host(url: str, base: str) -> bool:
    return (urlparse(url).hostname or "").lower() == (urlparse(base).hostname or "").lower()


def route_priority(url: str, status: int | None, content_type: str = "", source: str = "") -> tuple[int, list[str]]:
    lowered = url.lower()
    score = 5
    reasons: list[str] = []
    for marker, weight in ROUTE_HINTS.items():
        if marker in lowered:
            score += weight
            reasons.append(f"route marker: {marker}")
    if any(lowered.split("?", 1)[0].endswith(suffix) for suffix in STATIC_SUFFIXES):
        return 0, ["static asset"]
    if "?" in url:
        score += 15
        reasons.append("query parameters")
    if status in {401, 403}:
        score += 15
        reasons.append("protected response boundary")
    if status == 200:
        score += 5
    if "json" in content_type.lower() or "graphql" in lowered:
        score += 10
        reasons.append("structured/API response signal")
    if source:
        reasons.append(f"discovered from {source}")
    return min(score, 100), reasons


class RateLimiter:
    def __init__(self, requests_per_second: float) -> None:
        self.interval = 1.0 / max(0.1, requests_per_second)
        self.last = 0.0

    def wait(self) -> None:
        delay = self.interval - (time.monotonic() - self.last)
        if delay > 0:
            time.sleep(delay)
        self.last = time.monotonic()


class ReconEngine:
    def __init__(self, target: str, output: Path, depth: int, rate: float, headers: dict[str, str] | None = None) -> None:
        self.target = normalize(target)
        self.base = self.target
        self.output = output
        self.depth = max(1, min(depth, 3))
        self.limiter = RateLimiter(rate)
        self.headers = {"User-Agent": "Yteam-Recon/1.0 (authorized security assessment)", "X-Bug-Bounty": ATTRIBUTION, "Accept": "*/*"}
        self.headers.update(headers or {})
        self.headers["X-Bug-Bounty"] = ATTRIBUTION
        self.observations: list[HTTPObservation] = []
        self.routes: dict[str, dict] = {}
        self.assets: set[str] = {self.base}
        self.passive_assets: list[dict[str, str]] = []
        self.resolved_addresses: list[str] = []
        self.notes: list[str] = []
        self.context = ssl.create_default_context()
        self.localsolver = LocalSolver(rate)

    def request(self, url: str) -> tuple[HTTPObservation, str]:
        self.localsolver.wait()
        self.limiter.wait()
        started = time.monotonic()
        request = Request(url, headers=self.headers, method="GET")
        try:
            with urlopen(request, timeout=12, context=self.context) as response:
                body = response.read(1_048_576)
                elapsed = int((time.monotonic() - started) * 1000)
                content_type = response.headers.get("Content-Type", "")
                text = body.decode("utf-8", errors="replace")
                localsolver = self.localsolver.inspect(dict(response.headers.items()), text, response.status)
                title = re.search(r"(?is)<title[^>]*>(.*?)</title>", text)
                observation = HTTPObservation(
                    url=redact_url(url), status=response.status, content_type=content_type, length=len(body), elapsed_ms=elapsed,
                    title=re.sub(r"\s+", " ", title.group(1)).strip() if title else "",
                    redirect=redact_url(response.geturl()) if response.geturl() != url else "",
                    server=response.headers.get("Server", ""),
                    technologies=self.technologies(response.headers, text),
                    security_headers=self.security_headers(response.headers),
                    localsolver=localsolver.__dict__,
                )
                self.observations.append(observation)
                if localsolver.action in {"stop", "manual_review"}:
                    self.notes.append(f"LocalSolver {localsolver.action}: {localsolver.gate} detected at {redact_url(url)}; subsequent active probes were halted.")
                return observation, text
        except HTTPError as error:
            elapsed = int((time.monotonic() - started) * 1000)
            localsolver = self.localsolver.inspect(dict(error.headers.items()), "", error.code)
            observation = HTTPObservation(url=redact_url(url), status=error.code, content_type=error.headers.get("Content-Type", ""), length=0, elapsed_ms=elapsed, localsolver=localsolver.__dict__, error=redact_text(str(error)))
            self.observations.append(observation)
            if localsolver.action in {"stop", "manual_review"}:
                self.notes.append(f"LocalSolver {localsolver.action}: {localsolver.gate} detected at {redact_url(url)}; subsequent active probes were halted.")
            return observation, ""
        except (URLError, TimeoutError, OSError) as error:
            observation = HTTPObservation(url=redact_url(url), status=None, content_type="", length=0, elapsed_ms=int((time.monotonic() - started) * 1000), error=redact_text(str(error)))
            self.observations.append(observation)
            return observation, ""

    @staticmethod
    def technologies(headers: object, body: str) -> list[str]:
        values = " ".join(f"{key}:{value}" for key, value in getattr(headers, "items", lambda: [])()).lower() + " " + body[:10000].lower()
        fingerprints = {"cloudflare": "cloudflare", "nginx": "nginx", "apache": "apache", "next.js": "nextjs", "__next_data__": "nextjs", "react": "react", "graphql": "graphql", "wordpress": "wordpress", "fastapi": "fastapi", "laravel": "laravel"}
        return sorted(label for marker, label in fingerprints.items() if marker in values)

    @staticmethod
    def security_headers(headers: object) -> dict[str, str]:
        wanted = {
            "content-security-policy", "access-control-allow-origin", "access-control-allow-credentials",
            "access-control-allow-methods", "access-control-allow-headers", "x-frame-options",
            "strict-transport-security", "x-content-type-options", "referrer-policy", "permissions-policy",
            "cache-control", "location", "www-authenticate", "x-powered-by",
        }
        return {
            str(key).lower(): ", ".join(str(item) for item in value) if isinstance(value, list) else str(value)
            for key, value in getattr(headers, "items", lambda: [])()
            if str(key).lower() in wanted
        }

    def add_route(self, url: str, source: str, observation: HTTPObservation | None = None) -> None:
        if not url.startswith(("http://", "https://")) or not same_host(url, self.base):
            return
        if url in self.routes:
            self.routes[url]["sources"] = sorted(set(self.routes[url]["sources"] + [source]))
            return
        score, reasons = route_priority(url, observation.status if observation else None, observation.content_type if observation else "", source)
        self.routes[url] = {"url": url, "source": source, "sources": [source], "priority": score, "reasons": reasons, "status": observation.status if observation else None, "content_type": observation.content_type if observation else ""}

    def discover_passive_assets(self) -> None:
        """Collect CT names without actively probing them.

        CT output is discovery-only. Scope and the main agent decide whether a
        discovered hostname may become an active target.
        """
        hostname = (urlparse(self.base).hostname or "").lower()
        if not hostname or "." not in hostname:
            return
        try:
            ipaddress.ip_address(hostname)
            return
        except ValueError:
            pass
        request = Request(
            f"https://crt.sh/?q=%25.{hostname}&output=json",
            headers={"User-Agent": self.headers["User-Agent"], "X-Bug-Bounty": ATTRIBUTION},
        )
        try:
            with urlopen(request, timeout=10, context=self.context) as response:
                records = json.loads(response.read(2_000_000).decode("utf-8", errors="replace"))
        except (HTTPError, URLError, TimeoutError, OSError, json.JSONDecodeError):
            self.notes.append("Passive certificate-transparency discovery unavailable; no active fallback was attempted.")
            return
        names: set[str] = set()
        for record in records if isinstance(records, list) else []:
            for name in str(record.get("name_value", "")).splitlines():
                normalized = name.strip().lower().lstrip("*.")
                if normalized == hostname or normalized.endswith("." + hostname):
                    names.add(normalized)
        self.passive_assets = [
            {"host": name, "state": "discovered_passive", "active_probe": "not_attempted_until_scope_check"}
            for name in sorted(names)
        ]

    def resolve_base_dns(self) -> None:
        """Resolve the selected base host for inventory only; do not fan out."""
        hostname = urlparse(self.base).hostname
        if not hostname:
            return
        try:
            self.resolved_addresses = sorted({item[4][0] for item in socket.getaddrinfo(hostname, None, type=socket.SOCK_STREAM)})
        except socket.gaierror:
            self.notes.append(f"DNS resolution failed for the selected base host: {hostname}")

    def stage_probe(self, url: str, source: str) -> str:
        observation, body = self.request(url)
        self.add_route(url, source, observation)
        score, _ = route_priority(url, observation.status, observation.content_type, source)
        if score >= 20:
            self.notes.append(f"High-signal route candidate: {url} ({observation.status}, priority {score})")
        return body

    def run(self) -> dict:
        self.output.mkdir(parents=True, exist_ok=True)
        self.resolve_base_dns()
        root_observation, root_body = self.request(self.base)
        self.add_route(self.base, "target", root_observation)
        if self.localsolver.halted:
            root_body = ""
        parser = PageParser()
        if root_body:
            parser.feed(root_body)
            for raw in parser.links:
                self.add_route(urljoin(self.base + "/", raw), "html-link")
            for raw in parser.scripts:
                self.add_route(urljoin(self.base + "/", raw), "script-src")
            for form in parser.forms:
                self.add_route(urljoin(self.base + "/", form["action"] or "/"), f"form:{form['method']}")
            for raw in INLINE_ROUTE_RE.findall(root_body):
                self.add_route(urljoin(self.base + "/", raw), "inline-js-route")
        for path in DOC_PATHS:
            if self.localsolver.halted:
                break
            body = self.stage_probe(urljoin(self.base + "/", path.lstrip("/")), "well-known-or-api-path")
            if body and path.endswith("robots.txt"):
                for line in body.splitlines():
                    if line.lower().startswith(("allow:", "disallow:")):
                        value = line.split(":", 1)[1].strip()
                        if value.startswith("/") and value != "*":
                            self.add_route(urljoin(self.base + "/", value.lstrip("/")), "robots")
            if body and path.endswith("sitemap.xml"):
                for discovered in re.findall(r"(?is)<loc>\s*([^<]+?)\s*</loc>", body):
                    self.add_route(discovered.strip(), "sitemap")
        if not self.localsolver.halted:
            self.discover_passive_assets()
        intelligence_dir = self.output / "intelligence"
        intelligence_dir.mkdir(parents=True, exist_ok=True)
        intelligence_lines = []
        for item in self.observations:
            intelligence_lines.append(json.dumps({
                "kind": "observation", "observed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "target": self.base, "endpoint": redact_url(item.url), "method": "GET", "status": item.status,
                "response_length": item.length, "response_shape": item.content_type, "actor": "anonymous",
                "scope": "unknown", "tags": [], "source": "yteam-recon",
            }, sort_keys=True))
        (intelligence_dir / "observations.jsonl").write_text("\n".join(intelligence_lines) + ("\n" if intelligence_lines else ""), encoding="utf-8")
        frontier = [url for url in self.routes if url != self.base and self.routes[url]["source"] in {"html-link", "form:GET", "script-src"}]
        for current_depth in range(1, self.depth):
            next_frontier: list[str] = []
            for url in frontier[:30]:
                if self.localsolver.halted:
                    break
                if urlparse(url).path.lower().endswith(STATIC_SUFFIXES):
                    continue
                body = self.stage_probe(url, "crawl")
                if body and "html" in next((item.content_type for item in self.observations if item.url == url), "").lower():
                    page = PageParser()
                    page.feed(body)
                    for raw in page.links | page.scripts:
                        discovered = urljoin(url, raw)
                        if discovered not in self.routes and same_host(discovered, self.base):
                            self.add_route(discovered, "crawl-link")
                            next_frontier.append(discovered)
                    for raw in INLINE_ROUTE_RE.findall(body):
                        discovered = urljoin(url + "/", raw)
                        if discovered not in self.routes and same_host(discovered, self.base):
                            self.add_route(discovered, "crawl-inline-route")
                            next_frontier.append(discovered)
            frontier = next_frontier
        result = {
            "schema_version": 1,
            "engine": "yteam-recon",
            "target": self.base,
            "target_slug": slugify(self.base),
            "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "stages": ["target-baseline", "document-paths", "html-js-mining", "route-classification", "bounded-crawl"],
            "request_count": len(self.observations),
            "observations": [asdict(item) for item in self.observations],
            "routes": sorted(self.routes.values(), key=lambda item: (-item["priority"], item["url"])),
            "technology": sorted({tech for item in self.observations for tech in item.technologies}),
            "passive_assets": self.passive_assets,
            "resolved_addresses": self.resolved_addresses,
            "intelligence_observations": str(intelligence_dir / "observations.jsonl"),
            "notes": self.notes,
            "non_claims": ["Recon output is not vulnerability proof.", "Priority is triage guidance, not severity."],
            "localsolver": self.localsolver.summary(),
        }
        safe_result = redact_value(result)
        (self.output / "recon.json").write_text(json.dumps(safe_result, indent=2) + "\n", encoding="utf-8")
        (self.output / "routes.jsonl").write_text("".join(json.dumps(redact_value(item), sort_keys=True) + "\n" for item in result["routes"]), encoding="utf-8")
        (self.output / "recon_notes.md").write_text("# Yteam Recon Notes\n\n" + "\n".join(f"- {redact_text(note)}" for note in self.notes) + "\n\n## Non-claims\n\n- Recon output is not vulnerability proof.\n- Public routes and response differentials require validation.\n", encoding="utf-8")
        return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run", choices=("run",))
    parser.add_argument("--target", required=True)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--depth", type=int, default=2)
    parser.add_argument("--rate", type=float, default=1.0)
    parser.add_argument("--headers", help="JSON object of extra headers; values are never written to evidence")
    parser.add_argument("--run-id", help="Existing bb_pipeline run to update after recon")
    args = parser.parse_args()
    try:
        headers = json.loads(args.headers) if args.headers else {}
        if not isinstance(headers, dict):
            raise ValueError("--headers must be a JSON object")
        output = args.output or DEFAULT_OUTPUT / slugify(args.target)
        result = ReconEngine(args.target, output, args.depth, args.rate, {str(key): str(value) for key, value in headers.items()}).run()
        if args.run_id:
            sys.path.insert(0, str(ROOT / "scripts"))
            try:
                from bb_pipeline import advance, event

                event(args.run_id, "note", f"Deep recon completed: {result['request_count']} requests, {len(result['routes'])} routes, {len(result['passive_assets'])} passive assets.", "recon")
                advance(args.run_id, "recon", "completed")
                advance(args.run_id, "mapping", "active")
            finally:
                sys.path.remove(str(ROOT / "scripts"))
    except (ValueError, json.JSONDecodeError) as error:
        print(f"yteam_recon: {error}", file=sys.stderr)
        return 2
    print(json.dumps({"target": result["target"], "output": str(output), "requests": result["request_count"], "routes": len(result["routes"]), "technologies": result["technology"]}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
