# `client`

Typed Go SDK for the YTEAM HTTP/SSE API. It covers health/status, sessions and
messages, prompt/input lifecycle, models/tools/usage, permissions/questions,
compaction/export, and global or session event streams. Use `New` with the
server URL and optional bearer token, or `NewWithHTTPClient` for custom
timeouts/transports.

Typed responses include `Status`, `AgentState`, `Selection`, `ProviderUsage`,
`IntegrationStatus`, `SessionContext`, and `APIError`. `EventStream` follows
standard SSE framing, ignores comment/keepalive lines, supports multiline data,
and can be safely closed or consumed until `io.EOF`.
