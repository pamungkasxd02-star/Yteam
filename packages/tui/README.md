# `tui`

Terminal UI port boundary. The Go TUI provides English Home/Session routes,
raw ANSI/UTF-8 editing, grapheme-aware display offsets, pickers,
permission/question dialogs, command/file autocomplete, asynchronous prompt
execution, persistent prompt history, tracked paste parts, viewport paging,
and external-editor integration.

Prompt history follows OpenCode's current contract: JSONL below `YTEAM_HOME`,
at most 50 entries, malformed records skipped on replay, consecutive duplicate
entries suppressed, and Up/Down navigation restores the draft at the end.
Piped input uses the same command-first line loop, so `/help`, `/quit`, and
`/exit` never consume provider quota.

The command surface follows OpenCode names where the corresponding Go runtime
capability exists: `/models`, `/variants`, `/agents`, `/sessions`, `/resume`,
`/continue`, `/new`, `/clear`, `/fork`, `/rename`, `/export`, `/history`,
`/skills`, `/mcps`, `/lsp`, `/plugins`, `/usage`, `/editor`, and `/exit` with
`/quit` and `/q` aliases. Features without a corresponding runtime service
remain explicitly staged rather than being represented by no-op commands.
