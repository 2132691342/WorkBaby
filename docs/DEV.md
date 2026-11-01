# 开发 · DEV

## 环境

| 依赖 | 版本 | 用途 |
|---|---|---|
| Rust stable (MSVC) | ≥1.85 | 后端；Windows 上需 VS Build Tools（C++ 工作负载 + WebView2 SDK） |
| Node.js | ≥20 | 前端构建 |
| Rust target | x86_64-pc-windows-msvc | 默认 |

首次拉起前端：`cd frontend && npm install`。

## 目录速查

```
src-tauri/            壳（tauri.conf.json / main.rs）
crates/wb-*           后端各 crate（依赖方向见 AGENTS.md §2.2）
frontend/src          前端唯一源码目录
docs/                 文档（改行为必须同步）
assets/               打包进二进制的资源（内置技能、帮助文档与 MinGit 运行时）
```

## 日常命令

| 场景 | 命令 |
|---|---|
| 整体开发（壳+热更） | `npm run tauri dev`（frontend 目录内） |
| 仅后端编译检查 | `cargo check --workspace --all-targets` |
| 仅前端开发 | `npm run dev`（需把 `localStorage.wb_dev_port` 设成后端端口） |
| 后端测试 | `cargo test --workspace` |
| 单个子系统测试 | `cargo test -p wb-kernel` |
| 前端类型检查 | `cd frontend && npm run typecheck` |
| 全量门禁 | `cargo fmt --all -- --check` + `cargo clippy --workspace --all-targets -- -D warnings` + `cargo test --workspace` + 下方前端两道 |
| 只跑后端门禁 | 上面三条 cargo 命令 |
| 只跑前端门禁 | `npm run typecheck` + `npm run build`（frontend 目录内） |
| 只验一个 crate | 把 `--workspace` 换成 `-p wb-features`（crate 名即参数） |
| 慢速门禁（真实外部依赖） | `cargo test --workspace -- --ignored` |
| 出包 | `npm run tauri build`（frontend 目录内） |

`cargo xt`（测试）与 `cargo xchk`（编译检查）是 `.cargo/config.toml` 里的别名。

## 调试

- 日志：`{数据目录}\logs\`（按天分文件）；数据目录默认 `%APPDATA%\WorkBaby`。
- 纯浏览器调前端：壳未启动时，`api/shell.ts` 读 `localStorage.wb_dev_port` 连指定端口；
  用 `cargo run -p wb-api --bin wb-api-standalone` 单独起 HTTP 服务。
- SSE 调试：浏览器 DevTools 的 EventSource 面板；重连时观察 `last_event_id`
  （手动重连走查询参数，EventSource 原生重连走 Last-Event-ID 请求头）与 seq 重放。
- 内核测试用假 Provider：按脚本回放模型事件流，不起真网络。

## 约定提醒（写代码前扫一眼）

- domain 字段 `xxx_` 前缀；JSON 全 snake_case；ID 带 `SESS_/ENT_/MSG_/RUN_/TC_` 前缀
- 错误码先在 `docs/ERROR-CODES.md` 落档，再在 `wb-domain/src/error.rs` 声明常量
- 每文件 ≤500 行、连续注释 ≤3 行、struct 字段必带注释
- HTTP 只 GET/POST；删除走 `POST /xxx/delete/:id`
- 色值只在 `themes.css`；文案只在 i18n；图标只用 AppIcon
- 子进程只经 `wb-tools::exec` 启动
