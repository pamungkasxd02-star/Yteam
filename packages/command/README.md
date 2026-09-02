# `command`

OpenCode-compatible command registry. It discovers built-in `init`/`review`
templates and Markdown commands below `command/`, `commands/`, and
`.opencode/command(s)`. Frontmatter supports `description`, `agent`, `model`,
`variant`, and `subtask`; templates support `$1`–`$9` and `$ARGUMENTS`.
