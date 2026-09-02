# `tui`

Terminal UI port boundary. The Go TUI provides Home/Session routes, raw
ANSI/UTF-8 editing, pickers, permission/question dialogs, command/file
autocomplete, asynchronous prompt execution, and persistent prompt history.

Prompt history follows OpenCode's current contract: JSONL below `YTEAM_HOME`,
at most 50 entries, malformed records skipped on replay, consecutive duplicate
entries suppressed, and Up/Down navigation restores the draft at the end.
Piped input uses the same command-first line loop, so `/help`, `/quit`, and
`/exit` never consume provider quota.
