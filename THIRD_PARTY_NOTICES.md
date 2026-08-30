# Third-Party Notices

Yteam is an integration layer. The upstream source trees are not committed to
the public Yteam repository; `scripts/bootstrap_sources.py` fetches them into
`vendor/` after a clone. Each upstream checkout remains governed by its own
license and notices.

| Component | Source | License | Local notice after bootstrap |
|---|---|---|---|
| OpenCode | https://github.com/anomalyco/opencode | MIT | `vendor/opencode/LICENSE` |
| Hermes Agent | https://github.com/NousResearch/hermes-agent | MIT | `vendor/hermes-agent/LICENSE` |
| Cybermes | https://github.com/Zyrexnn/Cybermes | Apache-2.0 | `vendor/cybermes/LICENSE` |

Do not remove or replace those upstream notices when using the bootstrap
workflow. The Yteam integration files are licensed under the repository
`LICENSE`.
