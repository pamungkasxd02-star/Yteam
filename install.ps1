$ErrorActionPreference = "Stop"

$python = Get-Command py -ErrorAction SilentlyContinue
if (-not $python) { $python = Get-Command python -ErrorAction SilentlyContinue }
if (-not $python) {
    Write-Error "YTEAM requires Python 3.11, 3.12, or 3.13. Install Python and run this script again."
    exit 2
}

if (Test-Path (Join-Path $PSScriptRoot "install.py")) {
    & $python.Source (Join-Path $PSScriptRoot "install.py") @args
} else {
    $bootstrapUrl = "https://raw.githubusercontent.com/pamungkasxd02-star/Yteam/main/install.py"
    $code = (Invoke-WebRequest -UseBasicParsing $bootstrapUrl).Content
    & $python.Source -c $code -- @args
}
exit $LASTEXITCODE
