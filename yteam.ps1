$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$python = Join-Path $root "runtime\.venv\Scripts\python.exe"
$marker = Join-Path $root "runtime\quit.marker"
if (-not (Test-Path -LiteralPath $python)) {
    throw "YTEAM is not installed in this checkout. Run: python scripts\install_yteam.py"
}

while ($true) {
    if (Test-Path -LiteralPath $marker) { Remove-Item -LiteralPath $marker -Force }
    & $python (Join-Path $root "scripts\yteam_tui.py") @args
    $rc = $LASTEXITCODE
    if (Test-Path -LiteralPath $marker) {
        Remove-Item -LiteralPath $marker -Force
        exit 0
    }
    Write-Host ""
    Write-Host "[!] YTEAM keluar sendiri (rc=$rc) - restarting otomatis..."
    Write-Host "Tekan Ctrl+C berulang kali untuk berhenti."
    Start-Sleep -Seconds 2
}
