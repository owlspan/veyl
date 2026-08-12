; Inno Setup script for Veyl.
;
; Produces a Windows installer that places veyl.exe, optionally ships
; a private copy of the Go toolchain, adds a PATH entry, and associates
; .vy files with a right-click "Run with Veyl" verb.
;
; Do not run iscc against this directly. It expects a staging tree that
; installer\build.ps1 produces:
;
;     powershell -ExecutionPolicy Bypass -File installer\build.ps1
;
; That script builds veyl.exe, assembles a trimmed Go toolchain in
; dist\stage\go, and then calls iscc. Building by hand without the
; staging tree fails on the [Files] entries, which is the intended
; behaviour: a bundled installer missing its bundle is worse than none.

#define AppName "Veyl"
#define AppVersion "0.17"
#define AppPublisher "Veyl"
#define AppExeName "veyl.exe"

[Setup]
AppId={{8E3C1F42-7A55-4C0E-9D21-2B6F4A8E5C10}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=veyl-{#AppVersion}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
SetupIconFile=..\icons\veyl.ico
UninstallDisplayIcon={app}\{#AppExeName}
AppSupportURL=https://github.com/slightlyaboveaverageAIslop/veyl
AppUpdatesURL=https://github.com/slightlyaboveaverageAIslop/veyl/releases
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
Name: "compact"; Description: "Just Veyl - I already have Go installed"
Name: "custom";  Description: "Choose what to install"; Flags: iscustom

[Components]
Name: "core"; Description: "Veyl compiler, examples and documentation"; \
    Types: full compact custom; Flags: fixed
Name: "gotoolchain"; Description: "Go toolchain (required to compile - leave ticked unless you already have Go)"; \
    Types: full custom; ExtraDiskSpaceRequired: 185000000
Name: "vscode"; Description: "VS Code syntax highlighting"; Types: full custom
Name: "docs"; Description: "Tutorial, language reference and examples"; Types: full custom

[Tasks]
Name: "addtopath"; Description: "Add Veyl to PATH so 'veyl' works in any terminal"; \
    GroupDescription: "Setup:"
Name: "associate"; Description: "Associate .vy files, with an icon and a right-click ""Run with Veyl"""; \
    GroupDescription: "Setup:"
Name: "prompt"; Description: "Add a ""Veyl prompt"" shortcut (a terminal with Veyl already on PATH)"; \
    GroupDescription: "Setup:"
Name: "desktopicon"; Description: "Put that shortcut on the Desktop too"; \
    GroupDescription: "Setup:"; Flags: unchecked

[Files]
Source: "..\veyl.exe";      DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\icons\veyl.ico"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\README.md";       DestDir: "{app}"; Components: core; Flags: ignoreversion

Source: "..\SYNTAX.md";       DestDir: "{app}"; Components: docs; Flags: ignoreversion
Source: "..\TUTORIAL.md";     DestDir: "{app}"; Components: docs; Flags: ignoreversion
Source: "..\examples\*.vy";   DestDir: "{app}\examples"; Components: docs; Flags: ignoreversion

; Dropped straight into the extensions folder. VS Code scans that
; directory at startup, so there is no marketplace step and nothing to
; sideload by hand.
Source: "..\editors\vscode\*"; DestDir: "{%USERPROFILE}\.vscode\extensions\veyl-lang"; \
    Components: vscode; Flags: ignoreversion recursesubdirs createallsubdirs

; The private Go toolchain. It lives under {app}\go and is deliberately
; kept off PATH: findGo in toolchain.go locates it by position, so a
; developer's own Go installation is never shadowed by this one.
Source: "..\dist\stage\go\*"; DestDir: "{app}\go"; Components: gotoolchain; \
    Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
; A terminal that already has Veyl on PATH, whether or not the PATH
; task was ticked. This is what makes the install usable immediately
; instead of after a sign-out and back in.
Name: "{group}\Veyl prompt"; Filename: "{cmd}"; \
    Parameters: "/K ""set PATH={app};%PATH% && echo Veyl {#AppVersion} - try: veyl run examples\hello.vy && cd /d {app}"""; \
    IconFilename: "{app}\veyl.ico"; Tasks: prompt
Name: "{autodesktop}\Veyl prompt"; Filename: "{cmd}"; \
    Parameters: "/K ""set PATH={app};%PATH% && echo Veyl {#AppVersion} - try: veyl run examples\hello.vy && cd /d {app}"""; \
    IconFilename: "{app}\veyl.ico"; Tasks: desktopicon

; The interactive console, which is what most people will actually
; click. Kept first in the group for that reason.
Name: "{group}\Veyl console"; Filename: "{app}\{#AppExeName}"; \
    Parameters: "console"; WorkingDir: "{app}"; \
    IconFilename: "{app}\veyl.ico"
Name: "{autodesktop}\Veyl console"; Filename: "{app}\{#AppExeName}"; \
    Parameters: "console"; WorkingDir: "{app}"; \
    IconFilename: "{app}\veyl.ico"; Tasks: desktopicon

Name: "{group}\Learn Veyl (tutorial)"; Filename: "{app}\TUTORIAL.md"; Components: docs
Name: "{group}\Veyl language reference"; Filename: "{app}\SYNTAX.md"; Components: docs
Name: "{group}\Examples"; Filename: "{app}\examples"; Components: docs
Name: "{group}\Check the installation"; Filename: "{cmd}"; \
    Parameters: "/K ""{app}\{#AppExeName}"" doctor"; IconFilename: "{app}\veyl.ico"
Name: "{group}\Uninstall Veyl"; Filename: "{uninstallexe}"

[Registry]
; PATH entry. Per-user or machine-wide, matching how setup was run.
Root: HKA; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; \
    ValueData: "{olddata};{app}"; Tasks: addtopath; \
    Check: NeedsPathEntry(ExpandConstant('{app}'))

; .vy file association and the right-click verb.
Root: HKA; Subkey: "Software\Classes\.vy"; ValueType: string; ValueName: ""; \
    ValueData: "Veyl.Source"; Flags: uninsdeletevalue; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source"; ValueType: string; ValueName: ""; \
    ValueData: "Veyl source file"; Flags: uninsdeletekey; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\DefaultIcon"; ValueType: string; ValueName: ""; \
    ValueData: "{app}\veyl.ico"; Tasks: associate
; Double-clicking used to run the program straight away, which meant a
; console window that opened, printed, and closed faster than anyone
; could read it. It also assumed running was the only thing you might
; want, when building a standalone .exe is arguably more useful.
;
; The default verb now asks, and keeps the window open. The other two
; are on the right-click menu for when you already know.
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell"; ValueType: string; ValueName: ""; \
    ValueData: "open"; Tasks: associate

Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\open"; ValueType: string; ValueName: ""; \
    ValueData: "Open with Veyl"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\open"; ValueType: string; ValueName: "Icon"; \
    ValueData: "{app}\veyl.ico"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\open\command"; ValueType: string; ValueName: ""; \
    ValueData: """{app}\{#AppExeName}"" open ""%1"""; Tasks: associate

Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\run"; ValueType: string; ValueName: ""; \
    ValueData: "Run with Veyl"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\run"; ValueType: string; ValueName: "Icon"; \
    ValueData: "{app}\veyl.ico"; Tasks: associate
; Through `open --run` rather than `run` directly, so the window stays
; up long enough to read what the program printed.
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\run\command"; ValueType: string; ValueName: ""; \
    ValueData: """{app}\{#AppExeName}"" open ""%1"" --run"; Tasks: associate

Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\build"; ValueType: string; ValueName: ""; \
    ValueData: "Build .exe with Veyl"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\build"; ValueType: string; ValueName: "Icon"; \
    ValueData: "{app}\veyl.ico"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\build\command"; ValueType: string; ValueName: ""; \
    ValueData: """{app}\{#AppExeName}"" open ""%1"" --build"; Tasks: associate

[Run]
Filename: "{app}\{#AppExeName}"; Parameters: "doctor"; \
    Description: "Check the installation"; Flags: postinstall runasoriginaluser shellexec skipifsilent

[UninstallDelete]
; The Go toolchain writes nothing here, but Windows sometimes leaves
; empty directories behind after a recursive install is removed.
Type: dirifempty; Name: "{app}\go"
Type: dirifempty; Name: "{app}\examples"
Type: dirifempty; Name: "{app}"
Type: filesandordirs; Name: "{%USERPROFILE}\.vscode\extensions\veyl-lang"

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

{ Veyl compiles by handing generated code to the Go toolchain, so an
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
        MsgBox('Veyl is installed, but the Go toolchain was not bundled ' +
               'and no Go was found on this machine.' + #13#10#13#10 +
               'Veyl compiles your programs by handing generated code to ' +
               'the Go toolchain, so it cannot build anything yet.' + #13#10#13#10 +
               'Either install Go from https://go.dev/dl/ or run this ' +
               'installer again and leave the Go toolchain component ticked.',
               mbInformation, MB_OK);
    end;
  end;
end;
