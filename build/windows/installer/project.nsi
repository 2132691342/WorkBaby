Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
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
!include "LogicLib.nsh"

# ---------------------------------------------------------------------------
# 本文件是唯一可持久化定制的安装器脚本。
# build/windows/installer/wails_tools.nsh 每次 wails build 都会从模板重新生成，
# 写进去的改动会被冲掉；只有 project.nsi 会被保留。
# ---------------------------------------------------------------------------

# 开机自启的注册表位置，装/卸都要清干净，否则卸载后还会开机弹窗。
!define AUTOSTART_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

# 覆盖安装前必须先关掉运行中的实例。
# 托盘常驻让「窗口已经关了」不等于「进程已退出」——exe 仍被文件句柄占着，
# NSIS 覆写时报 "Error opening file for writing"，对用户是莫名其妙的失败。
# 只用 NSIS 内置指令：这个打包环境不带任何插件（nsExec 之类用不了）。
# taskkill 找不到目标返回 128、杀掉返回 0，据此决定要不要提示。
!macro killRunningInstance
    Exec 'taskkill /F /IM ${PRODUCT_EXECUTABLE} /T'
    ${If} $0 == 0
        DetailPrint "已关闭正在运行的 WorkBaby（它可能收在右下角托盘里）"
        ; 给 Windows 一点时间真正释放文件句柄，否则紧接着的覆写仍可能失败
        Sleep 1500
    ${EndIf}
!macroend

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

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
   !insertmacro killRunningInstance
FunctionEnd

Section
    # Disk space estimate: exe + ~161MB of bundled runtimes (in KB).
    # Without it the installer under-reports the required space and can fail mid-copy.
    AddSize 187000

    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    # The old instance can be relaunched while WebView2 is installing, so probe again
    # right before overwriting the exe.
    !insertmacro killRunningInstance

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # Bundled runtimes (Python / PowerShell) live next to the exe.
    # runtime.Manager unpacks them into the user directory on first launch and
    # prepends them to the PATH of exec / skillrun child processes.
    # Source dir is produced by scripts/copy-runtimes.ps1 into build/windows/runtimes.
    # The archives are already zip/tar.gz: keeping them uncompressed avoids paying
    # compress-then-decompress cost on ~161MB that is already compressed.
    #
    # NOTE: NSIS has no global "compress on" switch -- do not 'SetCompress on' here.
    # Scoping is via SetCompress off followed by SetCompress auto.
    SetOutPath "$INSTDIR\runtimes"
    SetCompress off
    File /r "..\runtimes\*.*"
    SetCompress auto
    SetOutPath $INSTDIR

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.setShellContext

    # 文件关联：wails_tools.nsh 里的 wails.associateFiles 是空宏（模板如此），
    # 因此直接调用同文件内的 APP_ASSOCIATE。FILECLASS 是注册表 ProgID，用 ASCII；
    # 中文只出现在用户能看到的名称里。
    !insertmacro APP_ASSOCIATE "md" "WorkBaby.Markdown" "Markdown 文档" "$INSTDIR\${PRODUCT_EXECUTABLE}" "用 WorkBaby 打开 Markdown" "$INSTDIR\${PRODUCT_EXECUTABLE} $\"%1$\""
    !insertmacro APP_ASSOCIATE "txt" "WorkBaby.Text" "文本文件" "$INSTDIR\${PRODUCT_EXECUTABLE}" "用 WorkBaby 打开文本" "$INSTDIR\${PRODUCT_EXECUTABLE} $\"%1$\""
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    !insertmacro killRunningInstance
    DeleteRegValue HKCU "${AUTOSTART_KEY}" "${INFO_PRODUCTNAME}"

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro APP_UNASSOCIATE "md" "WorkBaby.Markdown"
    !insertmacro APP_UNASSOCIATE "txt" "WorkBaby.Text"
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
