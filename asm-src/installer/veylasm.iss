; Inno Setup script for the Veyl assembly backend.
;
; This is the small one. The Go backend's installer ships a trimmed Go
; toolchain because that backend compiles by handing generated code to
; the Go compiler. This one compiles, assembles and links by itself, so
; there is nothing to bundle: one exe, a PATH entry, and the file
; associations.
;
; That is why there is no toolchain component, no ExtraDiskSpaceRequired
; and no post-install check that a toolchain exists. Every one of those
; is in veyl.iss for a reason that does not apply here.
;
; Build it with installer\build.ps1 rather than calling iscc directly,
; so veylasm.exe is built and stamped first:
;
;     powershell -ExecutionPolicy Bypass -File installer\build.ps1

#define AppName "Veyl (assembly backend)"
#define ShortName "veylasm"
#define AppVersion "0.18.0"
#define AppPublisher "Veyl"
#define AppExeName "veylasm.exe"

[Setup]
; Its own AppId, so it installs alongside the Go backend rather than
; upgrading over it. The two are meant to coexist: the Go one is the
; definition of the language, this one is the native compiler.
AppId={{B7D42A19-3E6C-4F80-A5D3-91C7E20B4F68}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={autopf}\Veyl-asm
DefaultGroupName=Veyl (assembly backend)
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=veylasm-{#AppVersion}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
SetupIconFile=..\..\icons\veyl.ico
UninstallDisplayIcon={app}\{#AppExeName}
AppSupportURL=https://github.com/owlspan/veyl
AppUpdatesURL=https://github.com/owlspan/veyl/releases
; Per-user by default, so no administrator prompt for the common case.
; PrivilegesRequired defaults to "admin", so without this line every
; install would open with a UAC prompt it does not need.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesInstallIn64BitMode=x64compatible
; The whole payload is one small exe and some text, so the wizard never
; sits there looking frozen and there is nothing to warn about.

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Components]
Name: "core"; Description: "The veylasm compiler"; Types: full compact custom; Flags: fixed
Name: "docs"; Description: "Examples and documentation"; Types: full custom

[Tasks]
Name: "addtopath"; Description: "Add veylasm to PATH so 'veylasm' works in any terminal"; \
    GroupDescription: "Setup:"
Name: "associate"; Description: "Add a right-click ""Compile with veylasm"" for .vl files"; \
    GroupDescription: "Setup:"
Name: "prompt"; Description: "Add a ""veylasm prompt"" shortcut (a terminal with veylasm already on PATH)"; \
    GroupDescription: "Setup:"
Name: "desktopicon"; Description: "Put that shortcut on the Desktop too"; \
    GroupDescription: "Setup:"; Flags: unchecked

[Files]
Source: "..\veylasm.exe";           DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\..\icons\veyl.ico";     DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "..\README.md";             DestDir: "{app}"; Components: core; Flags: ignoreversion

Source: "..\..\docs\SYNTAX.md";   DestDir: "{app}"; Components: docs; Flags: ignoreversion
Source: "..\..\docs\TUTORIAL.md"; DestDir: "{app}"; Components: docs; Flags: ignoreversion
Source: "..\examples\*.vl";       DestDir: "{app}\examples"; Components: docs; Flags: ignoreversion
Source: "..\examples\net\*.vl";   DestDir: "{app}\examples\net"; Components: docs; Flags: ignoreversion
Source: "..\examples\gui\*.vl";   DestDir: "{app}\examples\gui"; Components: docs; Flags: ignoreversion

[Icons]
; A terminal with veylasm on PATH whether or not the PATH task was
; ticked, so the install is usable without signing out and back in.
Name: "{group}\veylasm prompt"; Filename: "{cmd}"; \
    Parameters: "/K ""set PATH={app};%PATH% && echo veylasm {#AppVersion} - try: veylasm run examples\collatz.vl && cd /d {app}"""; \
    IconFilename: "{app}\veyl.ico"; Tasks: prompt
Name: "{autodesktop}\veylasm prompt"; Filename: "{cmd}"; \
    Parameters: "/K ""set PATH={app};%PATH% && echo veylasm {#AppVersion} - try: veylasm run examples\collatz.vl && cd /d {app}"""; \
    IconFilename: "{app}\veyl.ico"; Tasks: desktopicon

Name: "{group}\Examples"; Filename: "{app}\examples"; Components: docs
Name: "{group}\Veyl language reference"; Filename: "{app}\SYNTAX.md"; Components: docs
Name: "{group}\Learn Veyl (tutorial)"; Filename: "{app}\TUTORIAL.md"; Components: docs
Name: "{group}\Uninstall veylasm"; Filename: "{uninstallexe}"

[Registry]
; PATH. The two hives keep it in completely different places and HKA
; cannot paper over that: the per-user one is HKCU\Environment, the
; machine one is under Session Manager. There is no HKLM\Environment,
; and asking for one fails with "code 87, the parameter is incorrect",
; which names the API call rather than the mistake.
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; \
    ValueData: "{olddata};{app}"; Tasks: addtopath; \
    Check: not IsAdminInstallMode and NeedsPathEntry(ExpandConstant('{app}'))
Root: HKLM; Subkey: "SYSTEM\CurrentControlSet\Control\Session Manager\Environment"; \
    ValueType: expandsz; ValueName: "Path"; \
    ValueData: "{olddata};{app}"; Tasks: addtopath; \
    Check: IsAdminInstallMode and NeedsPathEntry(ExpandConstant('{app}'))

; No .vl association here, deliberately. The Go backend's installer
; claims the file type, and two installers fighting over which one owns
; .vl would mean whichever ran last wins. These are added verbs on the
; existing type instead, so both can be installed and either can be
; reached from the same right-click menu.
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\veylasmbuild"; ValueType: string; ValueName: "MUIVerb"; \
    ValueData: "Compile to .exe with veylasm"; Flags: uninsdeletekey; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\veylasmbuild"; ValueType: string; ValueName: "Icon"; \
    ValueData: "{app}\veyl.ico"; Tasks: associate
; Through cmd /K so the window stays up long enough to read what it
; said, whether that was a path or a list of errors.
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\veylasmbuild\command"; ValueType: string; ValueName: ""; \
    ValueData: "{cmd} /K """"{app}\{#AppExeName}"" build ""%1"""""; Tasks: associate

Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\veylasmrun"; ValueType: string; ValueName: "MUIVerb"; \
    ValueData: "Run with veylasm"; Flags: uninsdeletekey; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\veylasmrun"; ValueType: string; ValueName: "Icon"; \
    ValueData: "{app}\veyl.ico"; Tasks: associate
Root: HKA; Subkey: "Software\Classes\Veyl.Source\shell\veylasmrun\command"; ValueType: string; ValueName: ""; \
    ValueData: "{cmd} /K """"{app}\{#AppExeName}"" run ""%1"""""; Tasks: associate

[UninstallDelete]
Type: dirifempty; Name: "{app}\examples\net"
Type: dirifempty; Name: "{app}\examples\gui"
Type: dirifempty; Name: "{app}\examples"
Type: dirifempty; Name: "{app}"

[Code]
{ Where PATH lives depends on how setup was run. The check and the
  removal both have to agree with the [Registry] section, or setup
  edits one hive and uninstall looks in the other. }
function PathHive: Integer;
begin
  if IsAdminInstallMode then
    Result := HKEY_LOCAL_MACHINE
  else
    Result := HKEY_CURRENT_USER;
end;

function PathKey: string;
begin
  if IsAdminInstallMode then
    Result := 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment'
  else
    Result := 'Environment';
end;

{ Only append if it is not already there, so reinstalling does not grow
  PATH without bound. }
function NeedsPathEntry(Dir: string): Boolean;
var
  Existing: string;
begin
  if not RegQueryStringValue(PathHive, PathKey, 'Path', Existing) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + Uppercase(Dir) + ';', ';' + Uppercase(Existing) + ';') = 0;
end;

{ Uninstalling has to take the PATH entry back out. Inno deletes values
  it created outright, but this is an edit to a value that already
  existed and belongs to the user, so removing it means rewriting the
  string without our segment. Leaving it would mean every install and
  remove cycle grows PATH by one dead directory. }
procedure RemoveFromPath(Dir: string);
var
  Existing, Rebuilt, Segment: string;
  P: Integer;
begin
  if not RegQueryStringValue(PathHive, PathKey, 'Path', Existing) then
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
    RegDeleteValue(PathHive, PathKey, 'Path')
  else
    RegWriteExpandStringValue(PathHive, PathKey, 'Path', Rebuilt);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath(ExpandConstant('{app}'));
end;
