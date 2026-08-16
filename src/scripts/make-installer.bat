@echo off
rem Double-click this to build a Veyl installer.
rem
rem It exists because Windows will not run a .ps1 on double-click.
rem That is a deliberate security default, not something to work around
rem in the script itself. A .bat does run, so this one just calls the
rem PowerShell script with the flags it needs.
rem
rem This builds only. It does not commit, tag, or push, and it does not
rem install anything: the result is a setup .exe in dist\ that you then
rem run yourself. To cut a full release instead, use release.ps1
rem directly from a terminal:
rem
rem     powershell -ExecutionPolicy Bypass -File scripts\release.ps1 -Version 0.18 -Push

setlocal
rem The scripts live in src\scripts, and everything below expects
rem to run from src\ itself.
cd /d "%~dp0.."

echo Building the Veyl installer. This takes about two minutes.
echo.

powershell -NoProfile -ExecutionPolicy Bypass -File "installer\build.ps1"
set BUILD_EXIT=%ERRORLEVEL%

echo.
if not "%BUILD_EXIT%"=="0" (
    echo Build FAILED with exit code %BUILD_EXIT%.
    echo.
    echo If it complained that a file is in use, that is usually the
    echo virus scanner holding the staging tree open. Run it again.
) else (
    echo Done. The installer is in the dist folder:
    echo.
    dir /b dist\*-setup.exe
)

echo.
pause
