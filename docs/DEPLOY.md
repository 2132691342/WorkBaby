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

| 内容 | 打包方式 | 运行时行为 |
|---|---|---|
| 内置技能包 | `assets/skills/` 经 `include_dir` 打进 `wb-skills` | 解压到 `{数据目录}\skills\builtin`，按包指纹校验，不符即清空重解压 |
| 内置帮助文档 | `assets/docs/` 随二进制打包 | 经 `GET /docs` 提供 |
| 内置运行时 | `assets/runtime/` 两份归档经 `include_bytes` 打进 `wb-service` | 见下 |
| 图标与元数据 | `src-tauri/icons/`、`tauri.conf.json` 的 productName / identifier / version | — |

## 内置运行时

两份归档随安装包分发，**首次启动后台解压**到数据目录：

| 归档 | 解压到 | 内含 |
|---|---|---|
| `PortableGit.zip` | `runtime\PortableGit\` | Git Bash、git、GNU 工具 |
| `Python313.zip` | `runtime\python\` | Python 3.13 标准库与 pip |

- 启动**不等它落地**：解压在线程里跑，界面与端口先就绪，进度写 `Services.runtime`，
  `GET /runtime` 报 `resp_phase`（preparing / ready / failed）、`resp_done_count` 与失败原因
- **指纹是 zip 字节数**，写入各根目录的 `.pack_stamp`；不符即清空重解压（升级后旧文件不残留）
- **单件失败只告警**：对应工具退回系统 PATH 探测，其余两件照常可用
- **子进程 PATH 恒由 `runtime::child_path` 构造**（内置在前、系统在后），
  bash / git / python / context_mode 沙箱 / GitHub 克隆五个消费方共用同一份运行时
- 用户机器不装任何环境也能干活，且**不改动用户 PATH**——注入只作用于我们启动的子进程

## 安装形态与数据

| 项 | 值 |
|---|---|
| 安装范围 | per-user（NSIS `installMode: currentUser`），不需要管理员 |
| 打包目标 | nsis（x86_64-pc-windows-msvc） |
| 数据目录 | `%APPDATA%\WorkBaby\`（`sessions/` `settings.db` `models.json` `auth.json` `mcp.json` `skills/` `features/` `runtime/` `logs/`） |
| 升级 | 覆盖安装；数据目录不动 |
| 卸载 | 默认不删用户数据 |

WebView2 由 Windows 10/11 系统自带。

## 单实例与自启

- 单实例：`tauri-plugin-single-instance`；二次启动唤起已有窗口。
- 自启：`tauri-plugin-autostart`，用户在设置页开关（写 HKCU Run，不碰 HKLM）。

## 发布流程

推送即发布：把主干推到 GitHub 的 `main`，并把 `v1.0.0` 标签强制对齐到当前主干头。

### 门禁先于推送

推送前必须跑通五道门禁（与 CI 同一套）：

```bash
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cd frontend && npm run typecheck && npm run build
```

只改后端跑前三条，只改前端跑后两条；只动一个 crate 时把 `--workspace`
换成 `-p wb-xxx` 缩小验证面。依赖真实外部环境的慢速测试标了 `#[ignore]`，
显式 `cargo test --workspace -- --ignored` 才跑。

### 步骤

1. 把改动整理成一次语义提交，提交信息写清做了什么与为什么。
2. 推主干：`git push origin main`。
3. 把 `v1.0.0` 对齐到当前头并强制覆盖：
   `git tag -f v1.0.0 && git push -f origin v1.0.0`。

强制覆盖而不是新建递增标签，是因为本项目把 `v1.0.0` 当作主干稳定线——
任何一次通过门禁的 `main` 头都是合法的 1.0.0。需要保留历史快照时改用别的标签名，
不占用 `v1.0.0`。

### 两条流水线

| 工作流 | 触发 | 做什么 |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR 到 `main` | clippy（`-D warnings`）、fmt 检查、`cargo test`、前端 typecheck、前端 build |
| `.github/workflows/release.yml` | push `v*` 标签 | 校验 tag 版本与 `tauri.conf.json` 一致 → 构建安装包 → 创建 GitHub Release |

发布流水线的版本校验是硬门禁：tag 与 `tauri.conf.json` 的 `version` 不一致直接失败，
避免装出来的包与标签对不上。

版本号三处必须一致：根 `Cargo.toml` 的 `workspace.package.version`、
`src-tauri/Cargo.toml` 的 `version`、`src-tauri/tauri.conf.json` 的 `version`。

## 更新

当前走手动分发：下载新安装包覆盖安装，数据目录不受影响。
