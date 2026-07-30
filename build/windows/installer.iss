; QuotaDock Windows 설치 스크립트 (Inno Setup)
; 계획서 §13 준수: 사용자 권한 설치, 설치 경로 선택, 시작 메뉴, 제거 프로그램, 설정 유지
; 빌드: ISCC.exe /DAppVersion=x.y.z installer.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

#define AppName "QuotaDock"
#define AppPublisher "jungdosa"
#define AppExeName "quotadock.exe"
#define AppId "com.jungdosa.quotadock"

[Setup]
AppId={{A5F3C2E1-9D4B-4A7C-8E6F-1B2C3D4E5F60}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppVerName={#AppName} {#AppVersion}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
; 사용자 권한 설치 (§13: 사용자 권한 설치 우선)
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputDir=..\..\dist
OutputBaseFilename=QuotaDock-{#AppVersion}-win-x64-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=..\..\assets\icon.ico
UninstallDisplayIcon={app}\{#AppExeName}
UninstallDisplayName={#AppName}
AllowNoIcons=yes
; unsigned 빌드 허용 (§13)

; 앱이 시스템 언어를 자동 감지하므로 설치기도 묻지 않고 OS 언어를 따른다.
ShowLanguageDialog=no

[Languages]
; 설치기 언어는 Inno 공식 배포에 포함되고 ISCC 컴파일을 통과하는 것만 넣는다.
; 인도네시아어는 공식 .isl이 없으므로 앱만 지원하고 설치기는 영어로 폴백한다.
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"
Name: "german"; MessagesFile: "compiler:Languages\German.isl"
Name: "french"; MessagesFile: "compiler:Languages\French.isl"
Name: "italian"; MessagesFile: "compiler:Languages\Italian.isl"
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"
Name: "spanish"; MessagesFile: "compiler:Languages\Spanish.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "autostart"; Description: "Windows 시작 시 자동 실행 / Start with Windows"; GroupDescription: "시작 옵션 / Startup"; Flags: unchecked

[Files]
Source: "..\..\dist\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\README.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"
Name: "{group}\{cm:UninstallProgram,{#AppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon

[Registry]
; 자동 실행: 설치형은 사용자 범위 HKCU Run (§11, §13)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "{#AppName}"; ValueData: """{app}\{#AppExeName}"" --hidden"; Tasks: autostart; Flags: uninsdeletevalue

[Run]
Filename: "{app}\{#AppExeName}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; 제거 시 사용자 설정은 유지 (§11, §13). settings.json 을 삭제하지 않는다.
; %APPDATA%\QuotaDock 은 의도적으로 제거 목록에 넣지 않음.

[Code]
// 제거 시 자동 실행 레지스트리 정리는 uninsdeletevalue 로 처리됨.
// 설정 폴더는 보존한다.
