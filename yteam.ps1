$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
python (Join-Path $root "scripts\yteam_tui.py") @args
exit $LASTEXITCODE
