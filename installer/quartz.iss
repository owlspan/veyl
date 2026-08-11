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
SetupIconFile=..\icons\quartz.ico
UninstallDisplayIcon={app}\{#AppExeName}
AppSupportURL=https://github.com/slightlyaboveaverageAIslop/quartz
AppUpdatesURL=https://github.com/slightlyaboveaverageAIslop/quartz/releases
; Per-user by default, so no administrator prompt for the common case.
; This has to be stated: PrivilegesRequired defaults to "admin", so
; without the line below the comment above would simply be untrue, and
; every install would open with a UAC prompt it does not need.
; The dialog still lets someone install machine-wide if they want to.
PrivilegesRequired=lowest
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
Name: "vscode"; Description: "VS Code syntax highlighting"; Types: full custom
Name: "docs"; Description: "Tutorial, language reference and examples"; Types: full custom

[Tasks]
Name: "addtopath"; Description: "Add Quartz to PATH so 'quartz' works in any terminal"; \
    GroupDescription: "Setup:"
Name: "associate"; Description: "Associate .qz files, with an icon and a right-click ""Run with Quartz"""; \
    GroupDescription: "Setup:"
Name: "prompt"; Description: "Add a ""Quartz prompt"" shortcut (a terminal with Quartz already on PATH)"; \
    GroupDescription: "Setup:"
Name: "desktopicon"; Description: "Put that shortcut on the Desktop too"; \
    GroupDescription: "Setup:"; Flags: unchecked

[Files]
Source: "..\quartz.exe";      DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\icons\quartz.ico"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\README.md";       DestDir: "{app}"; Components: core; Flags: ignoreversion

Source: "..\SYNTAX.md";       DestDir: "{app}"; Components: docs; Flags: ignoreversion
Source: "..\TUTORIAL.md";     DestDir: "{app}"; Components: docs; Flags: ignoreversion
Source: "..\examples\*.qz";   DestDir: "{app}\examples"; Components: docs; Flags: ignoreversion

; Dropped straight into the extensions folder. VS Code scans that
; directory at startup, so there is no marketplace step and nothing to
; sideload by hand.
Source: "..\editors\vscode\*"; DestDir: "{%USERPROFILE}\.vscode\extensions\quartz-lang"; \
    Components: vscode; Flags: ignoreversion recursesubdirs createallsubdirs

; The private Go toolchain. It lives under {app}\go and is deliberately
; kept off PATH: findGo in toolchain.go locates it by position, so a
; developer's own Go installation is never shadowed by this one.
Source: "..\dist\stage\go\*"; DestDir: "{app}\go"; Components: gotoolchain; \
    Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
; A terminal that already has Quartz on PATH, whether or not the PATH
; task was ticked. This is what makes the install usable immediately
; instead of after a sign-out and back in.
Name: "{group}\Quartz prompt"; Filename: "{cmd}"; \
    Parameters: "/K ""set PATH={app};%PATH% && echo Quartz {#AppVersion} — try: quartz run examples\hello.qz && cd /d {app}"""; \
    IconFilename: "{app}\quartz.ico"; Tasks: prompt
Name: "{autodesktop}\Quartz prompt"; Filename: "{cmd}"; \
    Parameters: "/K ""set PATH={app};%PATH% && echo Quartz {#AppVersion} — try: quartz run examples\hello.qz && cd /d {app}"""; \
    IconFilename: "{app}\quartz.ico"; Tasks: desktopicon

Name: "{group}\Learn Quartz (tutorial)"; Filename: "{app}\TUTORIAL.md"; Components: docs
Name: "{group}\Quartz language reference"; Filename: "{app}\SYNTAX.md"; Components: docs
Name: "{group}\Examples"; Filename: "{app}\examples"; Components: docs
Name: "{group}\Check the installation"; Filename: "{cmd}"; \
    Parameters: "/K ""{app}\{#AppExeName}"" doctor"; IconFilename: "{app}\quartz.ico"
Name: "{group}\Uninstall Quartz"; Filename: "{uninstallexe}"

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
    ValueData: "{app}\quartz.ico"; Tasks: associate
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
Type: dirifempty; Name: "{app}\examples"
Type: dirifempty; Name: "{app}"
Type: filesandordirs; Name: "{%USERPROFILE}\.vscode\extensions\quartz-lang"

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

{ Uninstalling has to take the PATH entry back out. Inno removes
  registry values it created outright, but this one is an edit to a
  value that already existed and belongs to the user, so removing it
  means rewriting the string without our segment. Leaving it behind
  would mean every install-and-remove cycle grows PATH by one dead
  directory, which is the kind of mess that outlives the program. }
procedure RemoveFromPath(Dir: string);
var
  Existing, Rebuilt, Segment: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Existing) then
    exit;

  Rebuilt := '';
  Existing := Existing + ';';
  repeat
    P := Pos(';', Existing);
    Segment := Trim(Copy(Existing, 1, P - 1));
    Delete(Existing, 1, P);
    if (Segment <> '') and (CompareText(Segment, Dir) <> 0) then
    begin
      if Rebuilt <> '' then
        Rebuilt := Rebuilt + ';';
      Rebuilt := Rebuilt + Segment;
    end;
  until Existing = '';

  if Rebuilt = '' then
    RegDeleteValue(HKEY_CURRENT_USER, 'Environment', 'Path')
  else
    RegWriteExpandStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Rebuilt);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath(ExpandConstant('{app}'));
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
