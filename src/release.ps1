# Cuts a Veyl release: verify, stamp the version, commit, tag, build
# the installer, refresh the fallback snapshot, and optionally push.
#
#     powershell -ExecutionPolicy Bypass -File release.ps1 -Version 0.16
#     powershell -ExecutionPolicy Bypass -File release.ps1 -Version 0.16 -Push
#
# Nothing is committed until the whole suite passes, so a failed test
# leaves the working tree exactly as it was. -Push is opt-in because
# publishing is the one step that cannot be taken back.
#
# Add -SkipInstaller to cut a version without spending the two minutes
# on LZMA compression.

param(
    [Parameter(Mandatory = $true)][string]$Version,
    [switch]$Push,
    [switch]$SkipInstaller
)

$ErrorActionPreference = 'Stop'
$repo = $PSScriptRoot

function Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Ok($msg) { Write-Host "    $msg" -ForegroundColor Green }

Push-Location $repo
try {
    if ($Version -notmatch '^\d+\.\d+(\.\d+)?$') {
        throw "version should look like 0.16 or 0.16.1, got '$Version'"
    }
    $tag = "v$Version"
    if ((& git tag -l $tag)) { throw "tag $tag already exists" }

    # A dirty tree means the release would contain something nobody
    # looked at. Untracked files are fine; modified tracked ones are not.
    $dirty = & git status --porcelain --untracked-files=no
    if ($dirty) {
        throw "working tree has uncommitted changes:`n$dirty`nCommit or stash them first."
    }

    Step 'regenerating the icon'
    & go run ./tools/mkicon
    if ($LASTEXITCODE -ne 0) { throw 'mkicon failed' }

    Step "stamping version $Version"
    $driver = Join-Path $repo 'veyl.go'
    $text = [System.IO.File]::ReadAllText($driver)
    $stamped = [regex]::Replace($text, 'const Version = "[^"]*"', "const Version = `"$Version`"")
    if ($stamped -eq $text) { throw "could not find the Version constant in veyl.go" }
    [System.IO.File]::WriteAllText($driver, $stamped)

    $iss = Join-Path $repo 'installer\veyl.iss'
    $issText = [System.IO.File]::ReadAllText($iss)
    $issStamped = [regex]::Replace($issText, '#define AppVersion "[^"]*"', "#define AppVersion `"$Version`"")
    if ($issStamped -eq $issText) { throw "could not find AppVersion in veyl.iss" }
    [System.IO.File]::WriteAllText($iss, $issStamped)
    Ok "veyl.go and veyl.iss now say $Version"

    # ReadAllText/WriteAllText rather than Get-Content/Set-Content: the
    # latter round-trips UTF-8 through the ANSI codepage on PowerShell
    # 5.1 and silently mangles every non-ASCII character in the file.

    Step 'verifying'
    $unformatted = & gofmt -l @(& git ls-files '*.go')
    if ($unformatted) { throw "gofmt would change:`n$unformatted" }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed' }
    & go build -o veyl.exe .
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    Ok 'gofmt clean, vet clean, tests pass, builds'

    Step "committing and tagging $tag"
    & git add -A
    $msgFile = Join-Path $env:TEMP "veyl-release-$Version.txt"
    [System.IO.File]::WriteAllText($msgFile, "Release $tag`n")
    # -F rather than -m: PowerShell 5.1 mangles quotes passed to a
    # native exe, and a commit message is the usual place that bites.
    & git commit -F $msgFile --quiet
    if ($LASTEXITCODE -ne 0) { throw 'git commit failed' }
    & git tag $tag
    Ok "tagged $tag"

    if (-not $SkipInstaller) {
        Step 'building the installer'
        & powershell -ExecutionPolicy Bypass -File (Join-Path $repo 'installer\build.ps1')
        if ($LASTEXITCODE -ne 0) { throw 'installer build failed' }
    }

    Step 'refreshing the Working version snapshot'
    # The snapshot lives one level up, beside src\, so the repo root
    # holds only the working copy and the last known-good build.
    $snap = Join-Path (Split-Path -Parent $repo) 'Working version'
    if (Test-Path $snap) { Remove-Item -Recurse -Force $snap }
    New-Item -ItemType Directory -Force $snap | Out-Null
    # Via a file, not a pipe. A PowerShell pipeline carries objects and
    # re-encodes text, so piping a tar stream through one corrupts it -
    # tar reports "Failed to open '\\.\tape0'", which is it falling back
    # to a default device because it never saw an archive at all.
    $tarball = Join-Path $env:TEMP "veyl-snapshot-$Version.tar"
    & git archive -o $tarball HEAD
    if ($LASTEXITCODE -ne 0) { throw 'git archive failed' }
    & tar -x -f $tarball -C $snap
    if ($LASTEXITCODE -ne 0) { throw 'unpacking the snapshot failed' }
    Remove-Item $tarball -Force
    Push-Location $snap
    try {
        & go build -o veyl.exe .
        if ($LASTEXITCODE -ne 0) { throw 'the snapshot does not build' }
        & go test ./... | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'the snapshot does not pass its tests' }
    } finally { Pop-Location }
    # RESTORE.md is not tracked, so it does not survive git archive and
    # has to be written fresh each time.
    @"
# Working version - a known-good snapshot

Taken from $tag, commit $(& git rev-parse --short HEAD).

Verified when copied, not assumed: it builds, the whole test suite
passes, and veyl.exe here was built from exactly these sources.

To go back to it:

    git reset --hard $tag

This folder is gitignored, which is what lets it survive that command.
Written by release.ps1; see the repository CLAUDE.md for the details.
"@ | Out-File -FilePath (Join-Path $snap 'RESTORE.md') -Encoding utf8
    Ok 'snapshot verified'

    if ($Push) {
        Step 'pushing'
        & git push
        if ($LASTEXITCODE -ne 0) { throw 'git push failed' }
        & git push --tags
        if ($LASTEXITCODE -ne 0) { throw 'git push --tags failed' }
        Ok 'pushed'

        # The installer is gitignored, because a 39 MB binary rebuilt
        # every version would bloat the history beyond use. A GitHub
        # release is where it belongs: attached to the tag it was built
        # from, downloadable without cloning anything.
        if (-not $SkipInstaller) {
            $setup = Join-Path $repo "dist\veyl-$Version-setup.exe"
            if (-not (Test-Path $setup)) {
                Write-Host "    no installer at $setup, skipping the release" -ForegroundColor Yellow
            } else {
                $gh = @(
                    "$env:ProgramFiles\GitHub CLI\gh.exe",
                    "${env:ProgramFiles(x86)}\GitHub CLI\gh.exe",
                    "$env:LOCALAPPDATA\Programs\GitHub CLI\gh.exe"
                ) | Where-Object { Test-Path $_ } | Select-Object -First 1
                if (-not $gh) { $gh = (Get-Command gh -ErrorAction SilentlyContinue).Source }

                if (-not $gh) {
                    Write-Host "    gh not found, so no release was published." -ForegroundColor Yellow
                    Write-Host "    winget install GitHub.cli"
                } else {
                    & $gh auth status 2>&1 | Out-Null
                    if ($LASTEXITCODE -ne 0) {
                        Write-Host "    gh is not signed in, so no release was published." -ForegroundColor Yellow
                        Write-Host "    Run 'gh auth login' once, then:"
                        Write-Host "    gh release create $tag `"$setup`" --title `"Veyl $Version`" --generate-notes"
                    } else {
                        Step "publishing the $tag release"
                        # --generate-notes writes the changelog from the
                        # commits since the last tag, so the release notes
                        # are the commit messages rather than a second
                        # description nobody keeps in step.
                        & $gh release create $tag $setup --title "Veyl $Version" --generate-notes
                        if ($LASTEXITCODE -ne 0) { throw 'gh release create failed' }
                        Ok "published, installer attached"
                    }
                }
            }
        }
    } else {
        Write-Host "`nNot pushed. When ready:" -ForegroundColor Yellow
        Write-Host "    git push && git push --tags"
    }

    Write-Host "`n$tag is ready." -ForegroundColor Green
    Get-ChildItem (Join-Path $repo 'dist\*-setup.exe') -ErrorAction SilentlyContinue |
        ForEach-Object { Write-Host ("    {0}  ({1:N0} MB)" -f $_.Name, ($_.Length / 1MB)) }
} finally {
    Pop-Location
}
