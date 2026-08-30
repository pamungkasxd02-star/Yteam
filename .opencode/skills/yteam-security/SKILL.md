---
name: yteam-security
description: Use the unified Hermes security workflow for authorized QA and bounty testing.
---

# Yteam Security Integration

Use this skill when the user asks for authorized penetration testing, bug-bounty hunting, application security QA, triage, evidence capture, or report preparation inside this workbench.

## Runtime model

The upstream OpenCode TUI is a presentation client. The Yteam/Hermes runtime owns model calls, tools, memory, delegation, browser automation, image routing, and durable evidence. Cybermes is consumed directly through its local skills, knowledge, Go utilities, and report pipeline; do not add or depend on a second MCP protocol layer.

## Skill routing

Load only the narrowest matching playbooks, then combine them with `triage-validation`, `evidence-collection`, and `evidence-hygiene` before reporting. Typical routes are:

- recon and scope: `recon-and-methodology`, `recon-scope-triage`, `api-recon-and-docs`;
- authorization: `hunt-idor`, `api-authorization-and-bola`, `hunt-auth-bypass`, `hunt-session`;
- web input: `hunt-sqli`, `hunt-xss`, `hunt-ssrf`, `hunt-csrf`, `hunt-cors`;
- workflows: `hunt-business-logic`, `hunt-race-condition`, `hunt-oauth`, `hunt-graphql`;
- infrastructure: `hunt-cloud-misconfig`, `cloud-iam-deep`, `hunt-subdomain`;
- deliverables: `triage-validation`, `report-writing`, `bugcrowd-reporting`, `evidence-hygiene`.

## Emerging-bug intelligence

When repeated observations do not fit a known class, record a compact redacted observation in the Yteam intelligence ledger and run the local emerging-bug analyzer. Compare clean baselines, differential behavior, actor/tenant scopes, response fingerprints, and state transitions. Rank novelty and confidence separately, preserve provenance, and propose only a non-destructive next test. `hypothesis` is never equivalent to `verified`; use the normal seven-question gate before creating a finding.

## Vision fallback

When the model lacks native vision, keep the attachment in the evidence pack and let Hermes route it through auxiliary vision or `vision_analyze`. If that tool is unavailable, use text extraction/OCR or ask for a textual capture. Never infer pixels from a filename, thumbnail, or an unverified description.

## Non-negotiable classification

Treat SPA HTML shells, public API catalogs, OIDC discovery, ordinary 401/403 responses, versions, public Swagger, CORS wildcard without credentials, DNS-only SSRF, and pure enumeration as reconnaissance or negative evidence—not findings. A finding requires a reproducible security-boundary crossing and concrete accepted impact.

## Evidence

Use researcher-owned fixtures where credentials already exist, redact cookies/tokens and unrelated PII, preserve timestamps and request IDs, and put full artifacts under the D: target pack. Reports and PoCs must be in English.
