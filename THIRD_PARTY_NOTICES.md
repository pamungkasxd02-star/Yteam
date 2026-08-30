# Third-Party Notices

YTEAM is a native Python project. Its runtime does not bundle or import an
upstream agent checkout. The optional browser observer uses Camoufox, and the
model client speaks the OpenAI-compatible API exposed by the configured model
provider. Those dependencies are installed from `requirements.txt` and remain
under their respective licenses.

| Component | Source | License / terms |
|---|---|---|
| PyYAML | https://pypi.org/project/PyYAML/ | MIT |
| Camoufox | https://github.com/daijro/camoufox | Mozilla Public License 2.0 |
| FastAPI | https://github.com/fastapi/fastapi | MIT |
| Uvicorn | https://github.com/encode/uvicorn | BSD-3-Clause |
| Playwright Python | https://github.com/microsoft/playwright-python | Apache-2.0 |
| MCP Python SDK | https://github.com/modelcontextprotocol/python-sdk | MIT |

The YTEAM source files are licensed under the repository `LICENSE`. No vendor
checkout, generated runtime data, engagement evidence, or model credentials
are part of the public source tree.
