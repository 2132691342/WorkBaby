# 部署 · DEPLOY

## 出包

```bash
cd frontend && npm run build          # 先出前端 dist
npm run tauri build                   # tauri build：编译 release + 打包
```

| 产物 | 位置 |
|---|---|
| 独立 exe | `src-tauri/target/release/WorkBaby.exe` |
| NSIS 安装包 | `src-tauri/target/release/bundle/nsis/WorkBaby_x.y.z_x64-setup.exe` |

## 打包内容

- **内置技能包**：21 个技能目录经 `include_dir` 打进 `wb-skills`，
  首次启动解压到数据目录 `skills\builtin\`。
- **内置帮助文档**：`assets/docs\` 下的四篇随二进制打包，经 `/docs` 端点提供。
- **图标与元数据**：`src-tauri/icons/`、`tauri.conf.json` 的 productName/identifier/version。

## 安装形态与数据

| 项 | 值 |
|---|---|
| 安装范围 | per-user（NSIS `installMode: currentUser`），不需要管理员 |
| 打包目标 | nsis（x86_64-pc-windows-msvc） |
| 数据目录 | `%APPDATA%\WorkBaby\`（sessions/ settings.db models.json auth.json mcp.json skills/ features/ logs/） |
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
