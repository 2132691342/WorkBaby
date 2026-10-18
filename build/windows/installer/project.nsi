Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
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

ManifestDPIAware true

!include "MUI.nsh"
!include "LogicLib.nsh"

# ---------------------------------------------------------------------------
# 唯一可持久化定制的安装器脚本：wails_tools.nsh 每次 wails build 都会重新生成，
# 写进去的改动会被冲掉；只有 project.nsi 会被保留。
# ---------------------------------------------------------------------------

# 开机自启的注册表位置，装/卸都要清干净，否则卸载后还会开机弹窗。
!define AUTOSTART_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

# 本机数据目录：界面设置、日志、内置运行时、背景图、人设。
# 必须与 backend/runtime 的 Resolve 一致（%APPDATA%\WorkBaby）；设了 WORKBABY_HOME
# 的便携用法安装器不认——那种情况下数据由用户自己保管。
!define USER_DATA_DIR "$APPDATA\${INFO_PROJECTNAME}"

# 旧版 WebView2 profile 目录：Wails 默认把浏览器缓存写到 %APPDATA%\<exe 名>。
# 新版已挪进数据目录的 webview\ 子目录，这里只为清掉升级残留（纯缓存，直接删）。
!define LEGACY_WEBVIEW_DIR "$APPDATA\${PRODUCT_EXECUTABLE}"

# 覆盖安装前必须先关掉运行中的实例。
# 托盘常驻让「窗口已经关了」不等于「进程已退出」——exe 仍被文件句柄占着，
# NSIS 覆写时报 "Error opening file for writing"，对用户是莫名其妙的失败。
# 只用 NSIS 内置指令：这个打包环境不带任何插件（nsExec 之类用不了）。
# taskkill 找不到目标返回 128、杀掉返回 0，据此决定要不要等它释放句柄——
# 取退出码必须用 ExecWait（Exec 不写 $0，写成 Exec 等于这个判断从来没生效过）。
!macro killRunningInstance
    ExecWait 'taskkill /F /IM ${PRODUCT_EXECUTABLE} /T' $0
    ${If} $0 == 0
        DetailPrint "已关闭正在运行的 WorkBaby（它可能收在右下角托盘里）"
        ; 给 Windows 一点时间真正释放文件句柄，否则紧接着的覆写仍可能失败
        Sleep 1500
    ${EndIf}
!macroend

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
# 数据的去留用组件页的勾选框表达，不弹窗：
# 默认只有「安装」必选项（隐藏），「清空旧数据」默认不勾；
# 静默安装（/S）跳过页面，未勾选的 section 不会执行——无人值守永不删数据。
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

# 卸载同理：勾选「删除本机数据」才删，默认不勾，静默卸载（/S）保持不删。
!insertmacro MUI_UNPAGE_COMPONENTS
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

# ---------------------------------------------------------------------------
# 安装
# ---------------------------------------------------------------------------

# 可选项：清空旧数据。/o 前缀 = 默认不勾；文本里写清后果，勾了才真的删。
Section /o "清空旧数据（对话外的本机数据：设置、日志、内置运行时）" SEC_CLEAN_DATA
    RMDir /r "${USER_DATA_DIR}"
    RMDir /r "${LEGACY_WEBVIEW_DIR}"
    DetailPrint "已清空本机数据目录：${USER_DATA_DIR}"
SectionEnd

# 主安装（名字为空 = 隐藏，组件页不显示也不能取消勾选）。
Section "" SEC_CORE
    # Disk space estimate: the exe carries the embedded runtimes (~50MB) and is
    # the only file we install. Without this the installer under-reports required
    # space and can fail mid-copy.
    AddSize 140000

    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    # The old instance can be relaunched while WebView2 is installing, so probe again
    # right before overwriting the exe.
    !insertmacro killRunningInstance

    SetOutPath $INSTDIR

    !insertmacro wails.files

    # Bundled runtimes (Node / Python) are embedded in the exe by go:embed
    # and unpacked into the user data directory on first launch -- nothing else to
    # package here.

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

# 组件页的说明文字：勾选框旁边只有一行，后果写在描述区。
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SEC_CLEAN_DATA} "删除 ${USER_DATA_DIR}：界面设置、日志、内置运行时、背景图。不清空则安装后沿用现有设置。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Function .onInit
   !insertmacro wails.checkArchitecture
   !insertmacro killRunningInstance
FunctionEnd

# ---------------------------------------------------------------------------
# 卸载
# ---------------------------------------------------------------------------

# 可选项：删除本机数据。/o = 默认不勾，重装后接着用；静默卸载（/S）保持不删。
# 注意：对话、模型、技能等 PI 状态在 ~/.pi/agent，永远不从这里删。
Section /o "un.删除本机数据（设置、日志、内置运行时）" UNSEC_DATA
    RMDir /r "${USER_DATA_DIR}"
    DetailPrint "已删除本机数据目录：${USER_DATA_DIR}"
SectionEnd

Section "un." UNSEC_CORE
    !insertmacro wails.setShellContext

    !insertmacro killRunningInstance
    DeleteRegValue HKCU "${AUTOSTART_KEY}" "${INFO_PROJECTNAME}"

    # 旧版 WebView2 profile（%APPDATA%\<exe 名>）：纯缓存，卸载一律清掉；
    # 新版的 profile 在数据目录的 webview\ 里，随上面的勾选一起处理。
    RMDir /r "${LEGACY_WEBVIEW_DIR}"

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro APP_UNASSOCIATE "md" "WorkBaby.Markdown"
    !insertmacro APP_UNASSOCIATE "txt" "WorkBaby.Text"
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd

!insertmacro MUI_UNFUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${UNSEC_DATA} "删除 ${USER_DATA_DIR}：界面设置、日志、内置运行时。对话与模型配置属于 PI（~/.pi/agent），不会被删除。"
!insertmacro MUI_UNFUNCTION_DESCRIPTION_END
