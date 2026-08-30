# Publishing Yteam on GitHub

This project is designed to publish as one open-source Yteam repository while
keeping OpenCode, Hermes Agent, and Cybermes as reproducible upstream sources.

## Before publishing

1. Review `REQUIREMENTS.md`, `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`, and `THIRD_PARTY_NOTICES.md`.
2. Keep `vendor/SOURCES.md` current with the upstream URLs and revisions.
3. Do not publish `runtime/`, `.env`, `auth.json`, session databases, cookies,
   reports containing customer data, or unredacted evidence.
4. Run the test suite and source bootstrap from a clean clone.
5. Review `git status` and `git diff --cached` before the first push.
6. Run the repository secret scan against the staged tree before publishing.

The root `.gitignore` deliberately excludes nested upstream checkouts. They
are fetched after cloning by `scripts/bootstrap_sources.py`.

## Create the repository

From the Yteam project root:

```bash
git init
git add .
git status
git diff --cached --stat
git commit -m "Initial Yteam security platform"
git branch -M main
git remote add origin https://github.com/<account>/<repository>.git
git push -u origin main
```

The repository includes `.github/workflows/ci.yml`. GitHub Actions runs the
Python integration suite on supported Python versions and, in a separate job,
bootstraps the pinned upstream sources, runs Cybermes Go tests, installs the
OpenCode lockfile, and typechecks the OpenCode package. No target is probed by
CI and no model/provider credential is required by CI.

Do not use `git add -f vendor/...` unless you have deliberately reviewed the
license, size, and redistribution terms for every upstream source. The normal
workflow is to publish the integration layer and fetch dependencies at setup.

## Reproduce from a fresh clone

```bash
git clone https://github.com/<account>/<repository>.git yteam
cd yteam
python3 scripts/bootstrap_sources.py
```

On Windows use `python` instead of `python3`. The bootstrap performs shallow
checkouts and a sparse Cybermes checkout that avoids Windows path-length
failures while retaining the Cybermes engine, tests, skills, tools, docs,
metadata, and selected text-only JWT/auth knowledge sources. Follow the root
`README.md` and `REQUIREMENTS.md` for the runtime setup.

## Release checklist

```text
[ ] LICENSE and third-party attribution reviewed
[ ] THIRD_PARTY_NOTICES.md matches the bootstrap sources and upstream licenses
[ ] SECURITY.md contains a private reporting route
[ ] No credentials or private engagement artifacts are staged
[ ] runtime/ and local profiles are ignored
[ ] upstream source URLs/revisions recorded in vendor/SOURCES.md
[ ] Python tests pass
[ ] Cybermes Go tests pass from vendor/cybermes after bootstrap
[ ] Windows, macOS, and Linux setup instructions tested or marked pending
[ ] README explains authorized-use boundaries
[ ] No automatic report submission or destructive mode exposed
[ ] `.github/workflows/ci.yml` is present and uses read-only workflow permissions
[ ] Staged tree has been checked for credentials and private evidence
```
