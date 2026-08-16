# Builds the Veyl Windows installer.
#
#     powershell -ExecutionPolicy Bypass -File installer\build.ps1
#
# Three steps: build veyl.exe, assemble a trimmed copy of the Go
# toolchain under dist\stage\go, then compile the Inno Setup script.
#
# The trimmed toolchain is the reason this script exists. A full GOROOT
# is about 225 MB, but roughly a fifth of that is the Go project's own
# API listings, conformance tests, and stdlib test files, none of which
# are needed to compile someone's program. Dropping them is mechanical,
# so it should not be a thing anybody does by hand.

$ErrorActionPreference = 'Stop'

$repo = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $repo 'dist'
$stage = Join-Path $dist 'stage'
$goDest = Join-Path $stage 'go'

Push-Location $repo
try {
    # 1. The compiler itself.
    Write-Host '==> building veyl.exe' -ForegroundColor Cyan
    & go build -o veyl.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }

    # 2. The toolchain to bundle. GOROOT is asked of the Go that is
    #    building this, so the bundle always matches the compiler that
    #    was tested rather than whatever is installed elsewhere.
    $goRoot = (& go env GOROOT).Trim()
    if (-not (Test-Path $goRoot)) { throw "GOROOT does not exist: $goRoot" }
    Write-Host "==> staging Go toolchain from $goRoot" -ForegroundColor Cyan

    # Deleting the previous staging tree fails intermittently, and the
    # error blames "another process" without naming one. Nothing of ours
    # is running: it is the virus scanner, which opens every file in a
    # freshly written 177 MB tree and holds each handle briefly. Retrying
    # is the standard answer on Windows, because there is nothing to wait
    # on and no way to ask it to let go.
    if (Test-Path $goDest) {
        $removed = $false
        foreach ($attempt in 1..6) {
            try {
                Remove-Item -Recurse -Force $goDest -ErrorAction Stop
                $removed = $true
                break
            } catch {
                if ($attempt -eq 1) {
                    Write-Host "    staging tree is locked, waiting for the scanner" -ForegroundColor Yellow
                }
                Start-Sleep -Seconds ($attempt * 2)
            }
        }
        if (-not $removed) {
            throw "could not clear $goDest after several attempts. " +
                  "Close anything browsing it, or exclude dist\ from your virus scanner."
        }
    }
    New-Item -ItemType Directory -Force $goDest | Out-Null

    # robocopy exit codes 0-7 mean success; 8 and above are real errors.
    $excludeDirs = @(
        (Join-Path $goRoot 'api'),
        (Join-Path $goRoot 'test'),
        (Join-Path $goRoot 'doc'),
        (Join-Path $goRoot 'misc'),
        (Join-Path $goRoot 'pkg\obj'),
        'testdata'
    )
    & robocopy $goRoot $goDest /E /XD @excludeDirs /XF *_test.go `
        /NFL /NDL /NJH /NJS /NC /NS /MT:8 | Out-Null
    if ($LASTEXITCODE -ge 8) { throw "robocopy failed with $LASTEXITCODE" }

    # A bundle that cannot compile is the failure worth catching early,
    # so prove it before spending minutes on LZMA compression.
    Write-Host '==> verifying the staged toolchain' -ForegroundColor Cyan
    Copy-Item (Join-Path $repo 'veyl.exe') $stage -Force
    $savedPath = $env:PATH
    $env:PATH = "$env:SystemRoot\System32"
    try {
        $out = & (Join-Path $stage 'veyl.exe') run (Join-Path $repo 'examples\hello.vy')
        if ($LASTEXITCODE -ne 0) { throw "the staged toolchain could not compile hello.vy" }
        if ($out -notmatch 'Hello world') { throw "unexpected output from hello.vy: $out" }
    } finally {
        $env:PATH = $savedPath
    }
    $mb = (Get-ChildItem $goDest -Recurse -File | Measure-Object Length -Sum).Sum / 1MB
    Write-Host ("    staged toolchain compiles, {0:N0} MB" -f $mb) -ForegroundColor Green

    # 3. The installer.
    $iscc = @(
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1

    if (-not $iscc) {
        throw "ISCC.exe not found. Install Inno Setup: winget install JRSoftware.InnoSetup"
    }

    Write-Host '==> compiling the installer (this takes a few minutes)' -ForegroundColor Cyan
    & $iscc (Join-Path $PSScriptRoot 'veyl.iss')
    if ($LASTEXITCODE -ne 0) { throw "iscc failed with $LASTEXITCODE" }

    Get-ChildItem (Join-Path $dist '*-setup.exe') | ForEach-Object {
        Write-Host ("==> {0}  ({1:N0} MB)" -f $_.Name, ($_.Length / 1MB)) -ForegroundColor Green
    }
} finally {
    Pop-Location
}
