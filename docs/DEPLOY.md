# 部署 · DEPLOY

## 出包

```bash
cd frontend && npm run build          # 先出前端 dist
npm run tauri build                   # tauri build：编译 release + 打包
```

| 产物 | 位置 |
|---|---|
| 独立 exe | `target\release\workbaby.exe`（安装后为 `WorkBaby.exe`） |
| NSIS 安装包 | `target\release\bundle\nsis\WorkBaby_<版本>_x64-setup.exe` |

仓库是 Cargo workspace，产物统一落在仓库根 `target\release\`，不在 `src-tauri\target\`。

## 打包内容

- **内置技能包**：22 个技能目录经 `include_dir` 打进 `wb-skills`。
  每次启动按包指纹（递归统计的文件数 + 总字节数）校验数据目录里的解压结果，
  不符即清空重解压——升级与技能删减后旧文件不残留。
- **内置帮助文档**：`assets/docs/` 下的四篇随二进制打包，经 `/docs` 端点提供。
- **内置运行时**：`assets/runtime/` 两份归档经 `include_bytes` 打进 `wb-service`——
  `PortableGit.zip`（Git Bash + git + GNU 工具）与 `Python313.zip`（Python 3.13 标准库与 pip）。
  启动时按 zip 字节数指纹检测数据目录的 `runtime\PortableGit\` 与 `runtime\python\`，
  不符即清空重解压；单件失败只告警，对应工具退回系统 PATH 探测。
  子进程 PATH 恒由 `runtime::child_path` 构造（内置在前、系统在后），
  bash / git / python / context_mode 沙箱 / GitHub 克隆五个消费方共用同一份运行时，
  用户机器不装任何环境也能干活，且不改动用户 PATH。
- **图标与元数据**：`src-tauri/icons/`、`tauri.conf.json` 的 productName/identifier/version。

## 安装形态与数据

| 项 | 值 |
|---|---|
| 安装范围 | per-user（NSIS `installMode: currentUser`），不需要管理员 |
| 打包目标 | nsis（x86_64-pc-windows-msvc） |
| 数据目录 | `%APPDATA%\WorkBaby\`（sessions/ settings.db models.json auth.json mcp.json skills/ features/ runtime/ logs/） |
| 升级 | 覆盖安装；数据目录不动 |
| 卸载 | 默认不删用户数据 |

WebView2 由 Windows 10/11 系统自带。

## 单实例与自启

- 单实例：`tauri-plugin-single-instance`；二次启动唤起已有窗口。
- 自启：`tauri-plugin-autostart`，用户在设置页开关（写 HKCU Run，不碰 HKLM）。

## 版本与发布

版本号三处必须一致：根 `Cargo.toml` 的 `workspace.package.version`、
`src-tauri/Cargo.toml` 的 `version`、`src-tauri/tauri.conf.json` 的 `version`。

打 `v*` 形式的 tag 会触发发布流水线：构建安装包并创建 GitHub Release。
流水线会先校验 tag 版本与 `tauri.conf.json` 的 version 一致，不一致直接失败。

## 更新

当前走手动分发：下载新安装包覆盖安装，数据目录不受影响。
