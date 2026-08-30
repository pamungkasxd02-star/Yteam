# Yteam Platform Architecture

```text
OpenCode TUI
    ↓ OpenAI-compatible local bridge
Hermes Agent runtime
    ↓ one AssessmentContext
Yteam Control Plane
    ├── Policy / scope
    ├── EventBus
    ├── ArtifactStore
    ├── EngineRegistry
    └── DAG scheduler
          ├── recon + Cybermes tooling
          ├── bot-bypass gate classifier
          ├── decrypt/format analyzer
          ├── pentest/QA matrix
          ├── server-guard hardening checks
          ├── intelligence / emerging-bug engine
           ├── cross-run knowledge learning
           ├── hidden-surface route graph and trust-boundary planner
           └── evidence/report delivery
```

## Why this is one platform

All pillars share one context and policy. They do not independently decide
whether something is exploitable. Recon produces signals; the intelligence
engine produces hypotheses; validation produces evidence; triage decides
whether a finding is reportable.

The hidden-surface planner runs after recon and before validation. It turns
route metadata into a bounded review graph covering object references, sibling
endpoint inconsistencies, API-version drift, GraphQL/REST overlap,
URL-processing flows, authentication surfaces, and business-state families.
It carries prerequisites and stop signals for every check; it does not access
customer objects or promote metadata directly to a finding.

## Engine contracts

Each engine receives `AssessmentContext` and returns `EngineResult`. It may
write only inside its `ArtifactStore` root and must use the centralized
`Policy` before an operation. Engines communicate through structured state and
`EventBus` events, not hidden globals.

## Learning contract

The system learns verified, killed, blocked, and duplicate outcomes across
runs using signatures in the Yteam knowledge base. It does not store secrets,
cookies, passwords, customer PII, or raw sensitive responses. Unknown behavior
is represented as a hypothesis with provenance, confidence, novelty, suggested
track, and a safe next test.

## Extension points

Future contributors can add engines under `src/` and register them in the
control plane without changing the OpenCode or Hermes upstream checkouts.
Future adapters can consume `EventBus` events for a dashboard, CI result, or
local monitoring process while keeping the primary chat surface unchanged.
