Unicode true

####
# WorkBaby 的 NSIS 安装器脚本（手维护文件，不是生成物；约束见 README.md）。
# INFO_*、PRODUCT_EXECUTABLE、WAILS_INSTALL_SCOPE 等宏由
# build/windows/Taskfile.yml 的 makensis 命令行传入，wails_tools.nsh 只给未定义的宏填默认值。
# 手动调试本脚本时先跑一次 task build 产出 exe，再在本目录：
#   makensis -DINFO_PROJECTNAME=WorkBaby -DINFO_PRODUCTNAME=WorkBaby -DINFO_PRODUCTVERSION=1.0.0 -DARG_WAILS_AMD64_BINARY=..\..\..\bin\WorkBaby.exe project.nsi
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show # This will always show the installation details.

# 卸载前弹一次询问：是否连数据目录一起删。数据目录名固定为
# %LOCALAPPDATA%\WorkBaby（壳的布局决定），不从安装器宏推导。
Var UninstallDeleteData

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

# 卸载前弹一次询问：会话、MCP 配置、技能与设置都是用户的工作成果，
# 默认保留；删除必须是用户明确选的。
# 本文件保存为 UTF-8 with BOM，中文文案才不会被 makensis 按 ANSI 误读。
Function un.onInit
    MessageBox MB_YESNO|MB_ICONQUESTION "同时删除用户数据吗？$\n$\n会话记录、MCP 配置、技能与设置保存在 %LOCALAPPDATA%\WorkBaby。$\n选「否」保留数据，重新安装后可继续使用。$\n$\nAlso delete user data (sessions, MCP config, skills, settings) under %LOCALAPPDATA%\WorkBaby?" IDYES delete_data
    StrCpy $UninstallDeleteData "0"
    Goto ask_done
delete_data:
    StrCpy $UninstallDeleteData "1"
ask_done:
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # 随包资产只有内置技能包：assets\skills 整目录装进安装目录。
    # 运行时（git / node / python）不随包分发，装完探测系统已装的；
    # 少装 assets，装完的机器技能页就是空的。
    SetOutPath $INSTDIR\assets
    File /r "..\..\..\assets\*.*"
    SetOutPath $INSTDIR

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # WebView2 缓存可再生且绑定这一次安装：无论选什么都清。
    RMDir /r "$LOCALAPPDATA\WorkBaby\webview"

    # 用户数据只有用户选了才删。
    ${If} $UninstallDeleteData == "1"
        RMDir /r "$LOCALAPPDATA\WorkBaby"
    ${EndIf}

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
