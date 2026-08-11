; Inno Setup script for Quartz.
;
; Produces a Windows installer that places quartz.exe, optionally ships
; a private copy of the Go toolchain, adds a PATH entry, and associates
; .qz files with a right-click "Run with Quartz" verb.
;
; Do not run iscc against this directly. It expects a staging tree that
; installer\build.ps1 produces:
;
;     powershell -ExecutionPolicy Bypass -File installer\build.ps1
;
; That script builds quartz.exe, assembles a trimmed Go toolchain in
; dist\stage\go, and then calls iscc. Building by hand without the
; staging tree fails on the [Files] entries, which is the intended
; behaviour: a bundled installer missing its bundle is worse than none.

#define AppName "Quartz"
#define AppVersion "0.15"
#define AppPublisher "Quartz"
#define AppExeName "quartz.exe"

[Setup]
AppId={{8E3C1F42-7A55-4C0E-9D21-2B6F4A8E5C10}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=quartz-{#AppVersion}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
; Per-user by default, so no administrator prompt for the common case.
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesInstallIn64BitMode=x64compatible
LicenseFile=
; The bundled toolchain is large and slow to compress. Without this the
; wizard looks frozen while it decompresses.
SetupLogging=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Types]
Name: "full";    Description: "Everything, including the Go toolchain (recommended)"
Name: "compact"; Description: "Just Quartz — I already have Go installed"
Name: "custom";  Description: "Choose what to install"; Flags: iscustom

[Components]
Name: "core"; Description: "Quartz compiler, examples and documentation"; \
    Types: full compact custom; Flags: fixed
Name: "gotoolchain"; Description: "Go toolchain (required to compile — leave ticked unless you already have Go)"; \
    Types: full custom; ExtraDiskSpaceRequired: 185000000

[Tasks]
Name: "addtopath"; Description: "Add Quartz to PATH so 'quartz' works in any terminal"; \
    GroupDescription: "Setup:"
Name: "associate"; Description: "Associate .qz files with Quartz"; \
    GroupDescription: "Setup:"

[Files]
Source: "..\quartz.exe";    DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\SYNTAX.md";     DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\TUTORIAL.md";   DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\README.md";     DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\examples\*.qz"; DestDir: "{app}\examples"; Components: core; Flags: ignoreversion

; The private Go toolchain. It lives under {app}\go and is deliberately
; kept off PATH: findGo in toolchain.go locates it by position, so a
; developer's own Go installation is never shadowed by this one.
Source: "..\dist\stage\go\*"; DestDir: "{app}\go"; Components: gotoolchain; \
    Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\Learn Quartz (tutorial)"; Filename: "{app}\TUTORIAL.md"
Name: "{group}\Quartz language reference"; Filename: "{app}\SYNTAX.md"
Name: "{group}\Examples"; Filename: "{app}\examples"

[Registry]
; PATH entry. Per-user or machine-wide, matching how setup was run.
Root: HKA; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; \
    ValueData: "{olddata};{app}"; Tasks: addtopath; \
    Check: NeedsPathEntry(ExpandConstant('{app}'))

; .qz file association and the right-click verb.
Root: HKA; Subkey: "Software\Classes\.qz"; ValueType: string; ValueName: ""; \
    ValueData: "Quartz.Source"; Flags: uninsdeletevalue; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Quartz.Source"; ValueType: string; ValueName: ""; \
    ValueData: "Quartz source file"; Flags: uninsdeletekey; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Quartz.Source\DefaultIcon"; ValueType: string; ValueName: ""; \
    ValueData: "{app}\{#AppExeName},0"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Quartz.Source\shell\run"; ValueType: string; ValueName: ""; \
    ValueData: "Run with Quartz"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Quartz.Source\shell\run\command"; ValueType: string; ValueName: ""; \
    ValueData: """{app}\{#AppExeName}"" run ""%1"""; Tasks: associate

[Run]
Filename: "{app}\{#AppExeName}"; Parameters: "doctor"; \
    Description: "Check the installation"; Flags: postinstall runasoriginaluser shellexec skipifsilent

[UninstallDelete]
; The Go toolchain writes nothing here, but Windows sometimes leaves
; empty directories behind after a recursive install is removed.
Type: dirifempty; Name: "{app}\go"
Type: dirifempty; Name: "{app}"

[Code]
{ Only append to PATH if it is not already there, so repeated installs
  do not grow the variable without bound. }
function NeedsPathEntry(Dir: string): Boolean;
var
  Existing: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Existing) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + Uppercase(Dir) + ';', ';' + Uppercase(Existing) + ';') = 0;
end;

{ Quartz compiles by handing generated code to the Go toolchain, so an
  install with neither the bundled copy nor a Go on PATH cannot build
  anything. Say so during setup rather than letting the first compile
  fail later with something the user cannot act on. }
procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep = ssPostInstall then
  begin
    if not WizardIsComponentSelected('gotoolchain') then
    begin
      if not Exec('cmd.exe', '/C go version', '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
         or (ResultCode <> 0) then
        MsgBox('Quartz is installed, but the Go toolchain was not bundled ' +
               'and no Go was found on this machine.' + #13#10#13#10 +
               'Quartz compiles your programs by handing generated code to ' +
               'the Go toolchain, so it cannot build anything yet.' + #13#10#13#10 +
               'Either install Go from https://go.dev/dl/ or run this ' +
               'installer again and leave the Go toolchain component ticked.',
               mbInformation, MB_OK);
    end;
  end;
end;
