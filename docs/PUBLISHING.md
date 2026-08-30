# Publishing YTEAM on GitHub

YTEAM is published as a single native Python repository. The policy runtime,
event ledger, session store, skill registry, bounded recon pipeline, and
assessment DAG are the source of truth. The model endpoint is external, but no
external source checkout is required.

## Before publishing

1. Review `REQUIREMENTS.md`, `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`, and
   `THIRD_PARTY_NOTICES.md`.
2. Do not publish `runtime/`, `.env`, `auth.json`, session databases, cookies,
   reports containing customer data, or unredacted evidence.
3. Run the test suite from a clean clone.
4. Review `git status` and `git diff --cached` before every push.
5. Run the repository secret scan against the staged tree.

## Create or update the repository

From the YTEAM project root:

```bash
git add -A
git diff --cached --stat
git diff --cached --check
python -m compileall -q scripts src
python -m unittest discover -s tests -p "test_*.py" -v
git commit -m "feat: make YTEAM a standalone native platform"
git branch -M main
git push -u origin main
```

The GitHub Actions workflow runs the native Python suite on supported Python
versions. CI never probes a target and requires no model credential.

## Reproduce from a fresh clone

```bash
git clone https://github.com/pamungkasxd02-star/Yteam.git yteam
cd yteam
python3 scripts/install_yteam.py --skip-browser-download
python3 scripts/yteam_doctor.py --json
python3 -m unittest discover -s tests -p "test_*.py" -v
```

On Windows use `python` instead of `python3`.

## Release checklist

```text
[ ] LICENSE and direct dependency attribution reviewed
[ ] SECURITY.md contains a private reporting route
[ ] No credentials or private engagement artifacts are staged
[ ] runtime/ and local profiles are ignored
[ ] Python compile and tests pass
[ ] Windows, macOS, and Linux setup instructions are accurate
[ ] README explains authorized-use boundaries
[ ] No automatic report submission or destructive mode is exposed
[ ] CI uses read-only workflow permissions
[ ] Staged tree has been checked for credentials and private evidence
```
