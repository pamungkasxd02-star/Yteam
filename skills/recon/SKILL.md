---
name: yteam-recon
description: Map authorized web surfaces with bounded, read-only requests.
---

# YTEAM Recon Skill

Use this playbook to identify an authorized application's reachable surface
without treating metadata as a vulnerability.

## Procedure

1. Confirm the exact target and written scope before sending requests.
2. Establish one clean baseline with status, headers, content type, and a short
   redacted body summary.
3. Extract first-party links, scripts, forms, API paths, documentation paths,
   and object-reference shapes.
4. Keep passive names and archive URLs as leads until scope permits active use.
5. Rank concrete trust boundaries: identity, tenant, object, state transition,
   URL fetch, file parser, and privileged function.

## Verification

Recon is complete only when the output contains reproducible request metadata,
scope decision, route sources, and explicit non-claims. A 200 SPA shell,
public catalog, version, or ordinary 401/403 is not a finding.
