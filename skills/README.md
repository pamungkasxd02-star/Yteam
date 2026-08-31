# YTEAM Skills

This directory contains the reviewed, first-party playbooks used by the YTEAM
skill registry. A skill is a guidance and validation contract; it is not an
automatic exploit runner.

## Included skills

| Directory | Registry name | Purpose |
|---|---|---|
| `recon/` | `yteam-recon` | Scope-aware, bounded surface mapping |
| `authorization/` | `yteam-authorization` | IDOR/BOLA and role/tenant boundary review |
| `injection/` | `yteam-injection` | Safe input-boundary canaries |
| `reporting/` | `yteam-reporting` | Evidence, triage, and report quality |
| `runtime/` | `yteam-runtime` | Runtime, state, policy, and worker operation |

Each skill is stored as:

```text
skills/<directory>/SKILL.md
```

`catalog.json` contains portable metadata for these entries. The registry reads
only this repository's `skills/` directory; external checkouts and
machine-specific paths are not required or discovered automatically.

## Loading policy

- `safe_reference` skills can be loaded for normal guidance.
- `controlled` skills are still reviewed first-party content and are loaded by
  the engine for planning, subject to the active policy.
- `quarantined` content is metadata-only and its body is not loaded.

The registry computes a content hash and section map for every real `SKILL.md`.
The runtime resolver uses that hash to invalidate cached content when a skill
changes.

## Adding a skill

1. Create `skills/<name>/SKILL.md`.
2. Add front matter with `name` and `description`.
3. Keep the guidance evidence-first, scope-aware, and non-destructive.
4. Add or update a focused test when the skill affects registry behavior.
5. Run the development checks from the repository README.
