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
scripts/              验证脚本
assets/               打包进二进制的资源（内置技能与帮助文档）
```

## 日常命令

| 场景 | 命令 |
|---|---|
| 整体开发（壳+热更） | `npm run tauri dev`（frontend 目录内） |
| 仅后端编译检查 | `cargo check --workspace` |
| 仅前端开发 | `npm run dev`（需桌面壳已启动，端口走 localStorage dev 兜底） |
| 后端测试 | `cargo test --workspace` |
| 前端类型检查 | `cd frontend && npm run typecheck` |
| 全量验证 | `bash scripts/check.sh` |
| 慢速验证（真实外部依赖） | `bash scripts/check-slow.sh` |
| 格式与静态检查 | `cargo fmt --all -- --check && cargo clippy --workspace -- -D warnings` |
| 出包 | `npm run tauri build`（产物 `src-tauri/target/release/bundle/`） |

## 调试

- 日志：`{数据目录}\logs\`（info/warn/error 分文件）；数据目录默认 `%APPDATA%\WorkBaby`。
- 纯浏览器调前端：壳未启动时，`shell.ts` 读 `localStorage.wb_dev_port` 连指定端口；
  用 `cargo run -p wb-api --bin wb-api-standalone` 单独起 HTTP 服务。
- SSE 调试：浏览器 DevTools 的 EventSource 面板；seq 断档会看到 `chat:gap`。
- 内核单测假 Provider：`wb-kernel` 测试夹具按脚本回放 SSE 流，不起真网络。

## 约定提醒（写代码前扫一眼）

- domain 字段 `xxx_` 前缀；JSON 全 snake_case；ID 带 `SESS_/ENT_/MSG_/RUN_/TC_` 前缀
- 错误码先在 `docs/ERROR-CODES.md` 落档再使用
- 每文件 ≤500 行、连续注释 ≤3 行、struct 字段必带注释
- HTTP 只 GET/POST；删除走 `POST /xxx/delete/:id`
- 色值只在 `themes.css`；文案只在 i18n；图标只用 AppIcon
- `std::process` 只出现在 wb-tools 与 wb-features
