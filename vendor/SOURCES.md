# Upstream Sources

This integration uses unmodified upstream Git checkouts:

| Component | Repository | Branch | Initial checkout |
|---|---|---|---|
| OpenCode TUI and server | `https://github.com/anomalyco/opencode.git` | `dev` | `dc4449d` |
| Hermes Agent runtime | `https://github.com/NousResearch/hermes-agent.git` | `main` | `4209d37` |
| Cybermes intelligence/direct tooling | `https://github.com/Zyrexnn/Cybermes.git` | `main` | `8d8d968` |

The short revisions above identify the initial source snapshot used to create this integration. Refresh them after an intentional upstream update with:

```bash
git -C vendor/opencode rev-parse --short HEAD
git -C vendor/hermes-agent rev-parse --short HEAD
git -C vendor/cybermes rev-parse --short HEAD
```

OpenCode and Hermes Agent are MIT-licensed upstream projects. Cybermes is
licensed under Apache License 2.0. Keep the applicable upstream `LICENSE`
files and attribution when redistributing this workbench. See
`THIRD_PARTY_NOTICES.md` for the integration-level notice.
