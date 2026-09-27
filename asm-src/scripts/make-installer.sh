#!/bin/sh
# Builds the Windows installer on Linux, through wine. The same steps as
# installer/build.ps1: build veyl.exe, check every version string agrees
# with it, prove it compiles and runs a program, then run Inno Setup.
#
#     scripts/make-installer.sh
#
# Needs Go, wine able to run both 64-bit programs (veyl.exe) and 32-bit
# ones (ISCC.exe), and Inno Setup 6 installed into the wine prefix. ISCC
# is looked for under C:\InnoSetup and the usual Program Files folders;
# set ISCC to its Windows path to point somewhere else.
#
# The result is dist/veyl-<version>-setup.exe, as on Windows.

set -eu

cd "$(dirname "$0")/.."
repo=$(pwd)

echo '==> building veyl.exe'
GOOS=windows GOARCH=amd64 go build -o veyl.exe ./compiler

version=$(wine veyl.exe version 2>/dev/null | tr -d '\r')
want=$(echo "$version" | sed -n 's/^veyl \([^ ]*\).*/\1/p')
echo "    $version"

have=$(sed -n 's/^#define AppVersion "\(.*\)".*/\1/p' installer/veyl.iss | tr -d '\r')
ext=$(sed -n 's/^#define ExtVersion "\(.*\)".*/\1/p' installer/veyl.iss | tr -d '\r')
pkg=$(sed -n 's/^ *"version": "\(.*\)".*/\1/p' ../editors/vscode/package.json)
if [ "$want" != "$have" ] || [ "$want" != "$ext" ] || [ "$want" != "$pkg" ]; then
    echo "version mismatch: veyl.exe says $want, AppVersion $have, ExtVersion $ext," \
         "editors/vscode/package.json $pkg" >&2
    exit 1
fi

echo '==> checking the compiler builds a program'
probe=$(mktemp -d)
trap 'rm -rf "$probe"' EXIT
echo 'print("installer probe {2 + 2}")' > "$probe/probe.vl"
out=$(wine veyl.exe run "$(winepath -w "$probe/probe.vl")" 2>/dev/null | tr -d '\r')
if [ "$out" != "installer probe 4" ]; then
    echo "unexpected probe output: $out" >&2
    exit 1
fi
echo '    compiles and runs'

iscc=${ISCC:-}
if [ -z "$iscc" ]; then
    prefix=${WINEPREFIX:-$HOME/.wine}
    for d in "InnoSetup" "Program Files (x86)/Inno Setup 6" "Program Files/Inno Setup 6"; do
        if [ -f "$prefix/drive_c/$d/ISCC.exe" ]; then
            iscc="C:\\$(echo "$d" | tr / '\\')\\ISCC.exe"
            break
        fi
    done
fi
if [ -z "$iscc" ]; then
    echo 'ISCC.exe not found in the wine prefix. Install Inno Setup 6 there, or set ISCC.' >&2
    exit 1
fi

echo '==> compiling the installer'
mkdir -p dist
wine "$iscc" /Q "$(winepath -w "$repo/installer/veyl.iss")"
ls -l "dist/veyl-$want-setup.exe"
