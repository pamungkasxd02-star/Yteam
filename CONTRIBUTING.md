# Contributing to Yteam Platform

Thanks for helping build an open-source security research platform. We welcome
contributions that are **legal, authorized, and defensible**.

## Authorized-use contract

This project is for authorized security testing, bug bounty, QA, and defensive
engineering only. Any contribution that primarily enables unauthorized access,
abuse, spam, scraping-at-scale, or defeating protections without authorization
will be rejected. We keep every pillar usable for authorized research while
never shipping "here's how to attack anyone" tooling.

## How to contribute

1. **Fork** the repository.
2. **Create a branch** with a short name (no `feat/`-style prefix): e.g. `bot-detector-akamai`.
3. **Write focused code** with docstrings stating the authorized-use boundary.
4. **Add tests** under `tests/` (run with `python -m unittest discover -s tests -p "test_*.py" -v`).
5. **Keep secrets out** — never commit tokens, keys, or customer data.
6. **Open a PR** describing what changed and why it's within authorized use.

## Structure

```
src/
  core/          # shared scope-safe utilities
  bot_bypass/    # anti-bot gate classification (authorized testing)
  decrypt/       # encoded/signed payload analysis (authorized research)
  pentest_qa/    # pentest + QA checklist and matrix
  server_guard/  # hardening/exposure checks (read-only)
tests/           # unit tests
scripts/         # Yteam orchestration engines
docs/            # guides
```

## Standards

- **Authorized-first**: every module that touches a remote target documents its intended authorized use.
- **Read-only defaults**: probes default to non-destructive.
- **Scope-safe**: no credential stuffing, no DoS, no customer-object access, no cloud-resource claims.
- **Evidence-bound**: hypotheses are not findings; triage gate before reporting.
- **Cross-platform**: code must run on Windows, macOS, and Linux.

## Code of conduct

Be respectful and constructive. Do not use this project to attack anyone without authorization.
