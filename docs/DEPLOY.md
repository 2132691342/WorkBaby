# 部署 · DEPLOY

## 出包

```bash
cd frontend && npm run build       # 先出前端 dist
cd .. && npm run tauri build       # tauri build：编译 release + 打包
```

| 产物 | 位置 |
|---|---|
| 独立 exe | `src-tauri/target/release/WorkBaby.exe` |
| NSIS 安装包 | `src-tauri/target/release/bundle/nsis/WorkBaby_x.y.z_x64-setup.exe` |

## 打包内容

- **内置技能包**：21 个技能目录经 `include_dir` 打进 `wb-skills`，
  首次启动解压到数据目录 `skills\builtin\`（带版本标记，升级后重刷）。
- **图标与元数据**：`src-tauri/icons/`、`tauri.conf.json` 的 productName/identifier/version。
- WebView2：Windows 10/11 系统自带；安装包固定「下载缺失的 WebView2」策略。

## 安装形态与数据

| 项 | 值 |
|---|---|
| 安装范围 | per-user（NSIS `installMode: currentUser`），不需要管理员 |
| 数据目录 | `%APPDATA%\WorkBaby\`（sessions/ settings.db models.json auth.json mcp.json skills/ logs/） |
| 升级 | 覆盖安装；数据目录不动；技能包版本标记变更时重解压 |
| 卸载 | 默认不删用户数据；卸载勾选「删除我的数据」才清目录 |

## 单实例与自启

- 单实例：tauri-plugin-single-instance；二次启动唤起已有窗口并转发参数（文件路径）。
- 自启：tauri-plugin-autostart，用户在设置页开关（写 HKCU Run，不碰 HKLM）。

## 更新

tauri-plugin-updater + 清单托管；当前未发正式版，先走手动分发。
