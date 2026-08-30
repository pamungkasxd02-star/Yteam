---
name: yteam-injection
description: Test input boundaries with safe, evidence-first canaries.
---

# YTEAM Input Validation Skill

Use this playbook for SQL/NoSQL injection, XSS, SSRF, parser, upload, and
template hypotheses. Keep payloads minimal and stop before destructive impact.

## Procedure

1. Identify the parser and content type from a real application request.
2. Capture a clean baseline before each differential test.
3. Prefer a visible data/timing/body oracle; use OOB only with a unique,
   authorized receiver.
4. Attribute one result to one parameter and record negative controls.
5. Escalate only to the smallest non-destructive proof that crosses a security
   boundary.

## Verification

An echoed input, status change, DNS-only callback, encoded payload, or generic
error is not sufficient. The evidence must show parser execution plus concrete
confidentiality, integrity, or availability impact.
