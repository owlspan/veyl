param(
    [Parameter(Mandatory = $true)][string]$Version
)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent

$driver = Join-Path $root 'veyl.go'
$text = [System.IO.File]::ReadAllText($driver)
$stamped = [regex]::Replace($text, 'const Version = "[^"]*"', "const Version = `"$Version`"")
if ($stamped -eq $text) { throw "could not find Version constant in veyl.go" }
[System.IO.File]::WriteAllText($driver, $stamped)

$iss = Join-Path $root 'installer\veyl.iss'
$issText = [System.IO.File]::ReadAllText($iss)
$issStamped = [regex]::Replace($issText, '#define AppVersion "[^"]*"', "#define AppVersion `"$Version`"")
if ($issStamped -eq $issText) { throw "could not find AppVersion in veyl.iss" }
[System.IO.File]::WriteAllText($iss, $issStamped)

Write-Host "Stamped $Version in veyl.go and installer\veyl.iss"
