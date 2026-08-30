---
name: yteam-runtime
description: Operate the standalone YTEAM runtime and persistent session state.
---

# YTEAM Runtime Skill

The native runtime owns model selection, session history, event logging,
command routing, policy display, and the security pipeline. No upstream agent or
UI checkout is needed.

## Commands

- `/models` lists the live Zen Free catalog.
- `/model <id>` selects the next model.
- `/status` shows policy and session metadata.
- `/history` shows a bounded local summary.
- `/clear` starts a fresh local session.
- `/bb <authorized-target>` starts the scoped pipeline.
