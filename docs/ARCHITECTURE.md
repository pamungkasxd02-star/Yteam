# YTEAM Architecture

YTEAM combines the strongest architectural ideas observed in mature agent
systems while keeping the implementation first-party and local-first.

## Design principles

1. **One narrow waist** — every frontend and remote adapter talks to the same
   `YteamRuntime` command, session, model, event, and policy interfaces.
2. **Events are the UI contract** — the UI renders normalized runtime events;
   it does not inspect provider-specific response formats.
3. **Durable state is separate from live state** — SQLite WAL stores sessions,
   ordered messages, event sequences, and pending prompts; in-memory state is a
   cache and live view only.
4. **Memory is not model training** — proposals require verification before
   they become context. Unverified model output can never silently become a
   durable rule.
5. **Visibility is not authorization** — a tool can be hidden from the model,
   denied at execution, or paused for approval. These are separate decisions.
6. **Remote control is an adapter, not a second agent** — Telegram, Discord,
   and WhatsApp all call the same command gate and share the same audit trail.
7. **Security defaults fail closed** — read-only, authorized targets, low rate,
   no customer objects, no destructive operations, and no auto-submission.

## Component graph

```text
             Native TUI / CLI / Telegram / Discord / WhatsApp bridge
                              |
                    CommandRegistry + ControlPlane
                              |
                    +---------v----------+
                    |   YteamRuntime     |
                    | policy + lifecycle |
                    | commands + events  |
                    +----+----------+----+
                         |          |
               +---------v--+   +---v-------------+
               | Session DB |   | Event Store     |
               | SQLite     |   | ordered/replay  |
               +------+-----+   +---+-------------+
                      |             |
               +------v-------------v------+
               | Agent turn / context      |
               | memory + skills + tools   |
               +------+-------------+------+
                      |             |
             +--------v--+   +------v-------+
             | Provider  |   | Assessment   |
             | adapters  |   | DAG / recon  |
             +-----------+   +--------------+
```

## Event contract

All model and tool implementations should emit normalized events:

```text
turn.started
message.delta
reasoning.delta
tool.started
tool.progress
tool.completed
approval.requested
approval.resolved
usage.updated
turn.completed
provider.error
```

Each durable event has an aggregate ID, monotonically increasing sequence,
event ID, timestamp, type, and redacted payload. Live subscribers may be
lossy; the durable event store is replayable.

## Durable job execution

Assessment work is admitted to the `jobs` table before a network request is
made. A worker claims a queued job with a lease, persists the underlying
pipeline run ID, updates a heartbeat, and stores a bounded result/error. On
startup, stale running leases are returned to the queue. This makes the TUI a
replaceable client: closing it does not cancel the hunt, and reopening it can
render the same job/session state.

## Memory lifecycle

```text
conversation / operator lesson
          ↓
redacted proposal (pending)
          ↓ explicit verification + evidence reference
verified lesson
          ↓ query-scoped context injection
next model turn
```

The system never claims continuous neural learning. It performs auditable,
retrieval-based learning with provenance and a verification gate.

## UI model

The native terminal UI uses a full-screen `prompt_toolkit` application with an
OpenCode-style interaction model:

- onboarding view: centered logo and composer before the first turn;
- workspace view: transcript/composer on the left and a persistent information
  rail on the right;
- right rail: context estimate, MCP state, memory counters, working directory,
  model, and policy state;
- bottom composer: multiline input, prompt history, slash completion, command
  palette, and interrupt handling;
- footer: active-turn state, context percentage, and keyboard hints.

The implementation is first-party and does not import an upstream UI. The
`--plain` mode remains available for pipes, CI, and non-interactive terminals.

It intentionally has no browser build or global UI dependency. A future web UI
can consume the same JSON event/session contract without changing the core.

## Remote control security

Remote control is opt-in and disabled unless configured:

- actor IDs must be explicitly allowlisted per provider;
- webhook requests require an HMAC signature;
- `/bb` requires an exact target allowlist;
- `/quit` is never remotely executable;
- secrets are environment-only and never accepted as chat content;
- all denied and accepted commands are recorded in the event ledger;
- the listener binds to localhost by default.

WhatsApp requires an external provider/webhook bridge; YTEAM does not pretend
to implement the WhatsApp network protocol itself.
