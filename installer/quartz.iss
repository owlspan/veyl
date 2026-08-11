; Inno Setup script for Quartz.
;
; Builds a Windows installer that puts quartz.exe somewhere sensible,
; adds it to PATH, and associates .qz files with a right-click
; "Run with Quartz" verb.
;
; UNVERIFIED. Inno Setup is not installed on the development machine, so
; this has never been compiled or run. Treat it as a starting point, not
; as something known to work. Build it with:
;
;     iscc installer\quartz.iss
;
; from https://jrsoftware.org/isdl.php — then actually run the result
; before trusting it.
;
; It expects quartz.exe to already exist in the repository root:
;
;     go build -o quartz.exe .

#define AppName "Quartz"
#define AppVersion "0.12"
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
Compression=lzma
SolidCompression=yes
WizardStyle=modern
; Per-user by default, so no administrator prompt for the common case.
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesInstallIn64BitMode=x64compatible

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "addtopath"; Description: "Add Quartz to PATH so 'quartz' works in any terminal"; GroupDescription: "Setup:"
Name: "associate"; Description: "Associate .qz files with Quartz"; GroupDescription: "Setup:"

[Files]
Source: "..\quartz.exe";   DestDir: "{app}"; Flags: ignoreversion
Source: "..\SYNTAX.md";    DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md";    DestDir: "{app}"; Flags: ignoreversion
Source: "..\examples\*";   DestDir: "{app}\examples"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\Quartz language reference"; Filename: "{app}\SYNTAX.md"
Name: "{group}\Examples"; Filename: "{app}\examples"

[Registry]
; PATH entry. Per-user or machine-wide, matching how setup was run.
Root: HKA; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; \
    ValueData: "{olddata};{app}"; Tasks: addtopath; Check: NeedsPathEntry(ExpandConstant('{app}'))

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

{ Quartz shells out to the Go toolchain, so an install without Go is
  half an install. Say so during setup rather than letting the first
  compile fail with something cryptic. }
procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep = ssPostInstall then
  begin
    if not Exec('cmd.exe', '/C go version', '', SW_HIDE, ewWaitUntilTerminated, ResultCode) or (ResultCode <> 0) then
      MsgBox('Quartz is installed, but Go was not found on this machine.' + #13#10#13#10 +
             'Quartz compiles your programs by handing generated code to the Go ' +
             'toolchain, so it needs Go installed to build anything.' + #13#10#13#10 +
             'Install it from https://go.dev/dl/ and reopen your terminal.',
             mbInformation, MB_OK);
  end;
end;
