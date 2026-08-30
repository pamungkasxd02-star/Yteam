---
name: yteam-authorization
description: Validate object and function authorization across owned identities.
---

# YTEAM Authorization Skill

Use this playbook for IDOR, BOLA, tenant isolation, role boundaries, and
function-level access control. Do not use guessed customer identifiers as a
substitute for researcher-owned fixtures.

## Procedure

1. Record the owner, attacker identity, object identifier, and expected policy.
2. Replay the smallest read request under the second owned identity.
3. Compare body semantics, not just status code or response length.
4. Test a harmless write only when the program permits owned fixtures.
5. Check sibling routes, API versions, GraphQL resolvers, and indirect object
   references for inconsistent authorization.

## Verification

Require a cross-identity foreign record or meaningful owned-fixture state change
before calling the behavior a candidate. Empty arrays, redacted bodies, and
synthetic 404 responses remain negative evidence.
