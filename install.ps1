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
    $ref = if ($env:YTEAM_REF) { $env:YTEAM_REF } else { "codex/autonomy-core-v1" }
    $env:YTEAM_REF = $ref
    $bootstrapUrl = "https://raw.githubusercontent.com/pamungkasxd02-star/Yteam/refs/heads/$ref/install.py"
    $bootstrapPath = Join-Path $env:TEMP "yteam-bootstrap-$PID.py"
    Invoke-WebRequest -UseBasicParsing -Uri $bootstrapUrl -OutFile $bootstrapPath
    try {
        & $python.Source $bootstrapPath @args
    } finally {
        Remove-Item $bootstrapPath -Force -ErrorAction SilentlyContinue
    }
}
exit $LASTEXITCODE
