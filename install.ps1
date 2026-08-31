$ErrorActionPreference = "Stop"

# Native Windows bootstrap: clone once, then run the real installer locally.
# Safe to stream through `curl.exe | powershell -Command -`.
$repo = if ($env:YTEAM_REPOSITORY) { $env:YTEAM_REPOSITORY } else { "https://github.com/pamungkasxd02-star/Yteam.git" }
$ref = if ($env:YTEAM_REF) { $env:YTEAM_REF } else { "codex/autonomy-core-v1" }
$root = if ($env:YTEAM_HOME) { [IO.Path]::GetFullPath($env:YTEAM_HOME) } else { Join-Path $env:LOCALAPPDATA "Yteam" }

$git = Get-Command git.exe -ErrorAction SilentlyContinue
if (-not $git) { throw "Git tidak ditemukan. Install Git for Windows lalu jalankan installer lagi." }

if (-not (Test-Path (Join-Path $root "scripts\install_yteam.py"))) {
    if (Test-Path $root) { throw "Folder YTEAM ada tetapi bukan checkout yang valid: $root" }
    New-Item -ItemType Directory -Force -Path (Split-Path $root -Parent) | Out-Null
    & $git.Source clone --depth 1 --branch $ref $repo $root
    if ($LASTEXITCODE -ne 0) { throw "Gagal mengambil YTEAM dari GitHub." }
}

$pythonCommand = $null
$pythonArgs = @()
$candidates = @(
    @{ command = "py.exe"; args = @("-3.13") },
    @{ command = "py.exe"; args = @("-3.12") },
    @{ command = "py.exe"; args = @("-3.11") },
    @{ command = "python.exe"; args = @() },
    @{ command = "python3.exe"; args = @() }
)
foreach ($candidate in $candidates) {
    $found = Get-Command $candidate.command -ErrorAction SilentlyContinue
    if ($found) {
        & $found.Source @($candidate.args) -c "import sys; raise SystemExit(0 if (3,11) <= sys.version_info[:2] < (3,14) else 1)" 2>$null
        if ($LASTEXITCODE -eq 0) {
            $pythonCommand = $found.Source
            $pythonArgs = $candidate.args
            break
        }
    }
}
if (-not $pythonCommand) { throw "Python 3.11, 3.12, atau 3.13 tidak ditemukan." }

$installer = Join-Path $root "scripts\install_yteam.py"
& $pythonCommand @pythonArgs $installer @args
exit $LASTEXITCODE
