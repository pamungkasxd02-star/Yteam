$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
python (Join-Path $root "scripts\hermes_opencode.py") @args
exit $LASTEXITCODE
