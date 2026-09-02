# `client`

Typed Go SDK for the YTEAM HTTP/SSE API. It covers health/status, sessions and
messages, prompt/input lifecycle, models/tools/usage, permissions/questions,
compaction/export, and global or session event streams. Use `New` with the
server URL and optional bearer token, or `NewWithHTTPClient` for custom
timeouts/transports.
