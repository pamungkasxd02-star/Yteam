$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$python = Join-Path $root "runtime\.venv\Scripts\python.exe"
if (-not (Test-Path -LiteralPath $python)) {
    throw "YTEAM is not installed in this checkout. Run: python scripts\install_yteam.py"
}
& $python (Join-Path $root "scripts\yteam_tui.py") @args
exit $LASTEXITCODE
