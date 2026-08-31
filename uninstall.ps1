$ErrorActionPreference = "Stop"
$python = Get-Command py -ErrorAction SilentlyContinue
if (-not $python) { $python = Get-Command python -ErrorAction SilentlyContinue }
if (-not $python) { Write-Error "Python is required to uninstall YTEAM from a checkout."; exit 2 }
& $python.Source (Join-Path $PSScriptRoot "scripts\yteam_uninstall.py") @args
exit $LASTEXITCODE
