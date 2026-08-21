param(
    [Parameter(Mandatory = $true)][string]$Version
)

# Writes $Version into every file that states one.
#
# There are five, and until now this script knew about two. The other
# three drifted: SYNTAX.md still said 0.16 and the "known limitations"
# section said 0.14, the README said 0.16, and the VS Code extension had
# been stuck at 0.12.0 since it was written, while the compiler itself
# said 0.17.02. Anything that has to be remembered gets forgotten, so
# every one of them is stamped here and a miss is a hard error.

$ErrorActionPreference = 'Stop'
$src = Split-Path $PSScriptRoot -Parent
$repo = Split-Path $src -Parent

# The extension manifest needs strict semver, so 0.17.02 has to lose the
# leading zero on the patch to become 0.17.2.
$semver = ($Version -split '\.' | ForEach-Object { [int]$_ }) -join '.'

$targets = @(
    @{ Path = Join-Path $src  'compiler\veyl.go';                Pattern = 'const Version = "[^"]*"';        Replace = "const Version = `"$Version`"" },
    @{ Path = Join-Path $src  'installer\veyl.iss';              Pattern = '#define AppVersion "[^"]*"';     Replace = "#define AppVersion `"$Version`"" },
    @{ Path = Join-Path $src  'editors\vscode\package.json';     Pattern = '"version": "[^"]*"';             Replace = "`"version`": `"$semver`"" },
    @{ Path = Join-Path $repo 'docs\SYNTAX.md';                  Pattern = '\*\*Version [^*]*\*\*';          Replace = "**Version $Version**" },
    @{ Path = Join-Path $repo 'docs\SYNTAX.md';                  Pattern = 'what v[0-9][^ ]* does not do';   Replace = "what v$Version does not do" },
    @{ Path = Join-Path $repo 'README.md';                       Pattern = '\*\*Status:\*\* v[^.]*\.[^.]*\.?[0-9]*\.'; Replace = "**Status:** v$Version." },
    @{ Path = Join-Path $repo 'README.md';                       Pattern = 'what v[0-9][^ ]* does not do';   Replace = "what v$Version does not do" }
)

foreach ($t in $targets) {
    if (-not (Test-Path $t.Path)) { throw "no such file to stamp: $($t.Path)" }
    $text = [System.IO.File]::ReadAllText($t.Path)
    $stamped = [regex]::Replace($text, $t.Pattern, $t.Replace)
    if ($stamped -eq $text) {
        throw "nothing matched /$($t.Pattern)/ in $($t.Path). Either the version is already $Version, or the line it lives on was reworded and this script needs updating."
    }
    [System.IO.File]::WriteAllText($t.Path, $stamped)
    Write-Host "    stamped $(Split-Path $t.Path -Leaf)"
}

Write-Host "Stamped $Version across $($targets.Count) places (extension: $semver)"
