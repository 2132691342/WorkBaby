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
src-tauri/            壳（tauri.conf.json / main.rs / lib.rs / cmds.rs）
crates/wb-*           后端各 crate（依赖方向见 AGENTS.md §2.2）
frontend/src          前端唯一源码目录
docs/                 文档（改行为必须同步）
assets/               打包进二进制的资源（内置技能、帮助文档与 PortableGit / Python 运行时）
```

## 日常命令

| 场景 | 命令 |
|---|---|
| 整体开发（壳 + 热更） | `npm run tauri dev`（frontend 目录内） |
| 仅后端编译检查 | `cargo check --workspace --all-targets`（别名 `cargo xchk`） |
| 仅前端开发 | `npm run dev`（需把 `localStorage.wb_dev_port` 设成后端端口） |
| 后端测试 | `cargo test --workspace`（别名 `cargo xt`） |
| 单个子系统测试 | `cargo test -p wb-kernel` |
| 前端类型检查 | `cd frontend && npm run typecheck` |
| 前端构建 | `cd frontend && npm run build` |
| 全量门禁 | `cargo fmt --all -- --check` + `cargo clippy --workspace --all-targets -- -D warnings` + `cargo test --workspace` + 前端 typecheck + build |
| 只跑后端门禁 | 上面三条 cargo 命令 |
| 只跑前端门禁 | `npm run typecheck` + `npm run build`（frontend 目录内） |
| 只验一个 crate | 把 `--workspace` 换成 `-p wb-features`（crate 名即参数） |
| 慢速门禁（真实外部依赖） | `cargo test --workspace -- --ignored` |
| 出包 | `npm run tauri build`（frontend 目录内） |

`cargo xt` / `cargo xchk` 是 `.cargo/config.toml` 里的别名。

## 测试布局

测试集中在各 crate 的 `tests/` 目录，一个 crate 一个测试文件，
文件内按 `// ---- 场景名 ----` 分节。只守值得守的东西：

| 文件 | 守什么 |
|---|---|
| `wb-domain/tests/messages_test.rs` | 每个业务码都有中英默认文案，未登记的码不冒充已收录 |
| `wb-pkg/tests/pkg_test.rs` | 路径越界判定；程序探测不起子进程 |
| `wb-provider/tests/provider_test.rs` | 工具调用分片累积与定稿、网关夹具差异（空 `finish_reason` / 断流）、停止原因映射、错误体提取 |
| `wb-kernel/tests/kernel_test.rs` | 一轮 run 的事件顺序与回放契约、并发分组、悬空调用补齐、压缩与活状态回注、分支编辑、子代理隔离、请求展开 |
| `wb-tools/tests/tools_test.rs` | 写 / 改工具必须回报改动文件与成对差异行 |
| `wb-features/tests/features_test.rs` | 待办依赖图、回答板一次性投递、记忆 FTS5 与密钥扫描、整合守恒、目标续跑状态机、多搜索源解析、LSP 路由与编辑 |
| `wb-service/tests/service_test.rs` | 记忆与配置读写、面板写入共用同一份落盘与状态机、工作区热切换、子代理观测、SSE 载荷契约 |
| `wb-skills/tests/skills_test.rs` | frontmatter 边界与内置技能包完整性 |
| `wb-git/tests/git_test.rs` | porcelain 解析（快）；真仓库全链路（`#[ignore]`） |

假 Provider / 假工具这类共享夹具放 `tests/common/mod.rs`（`wb-kernel` 与 `wb-service` 各一份）。

不写：只验证常量等于自身、只覆盖单一分支的断言，以及框架行为的复述。

## 调试

- 日志：`{数据目录}\logs\`（按天分文件）；数据目录默认 `%APPDATA%\WorkBaby`。
- 纯浏览器调前端：壳未启动时，`api/shell.ts` 读 `localStorage.wb_dev_port` 连指定端口；
  用 `cargo run -p wb-api --bin wb-api-standalone` 单独起 HTTP 服务。
- SSE 调试：浏览器 DevTools 的 EventSource 面板；重连时观察 `last_event_id`
  （手动重连走查询参数，EventSource 原生重连走 `Last-Event-ID` 请求头）与 seq 重放。
- 内核测试用假 Provider：按脚本回放模型事件流，不起真网络。

## 约定提醒（写代码前扫一眼）

- domain 字段 `xxx_` 前缀；JSON 全 snake_case；ID 带 `SESS_/ENT_/MSG_/RUN_/TC_/ASK_` 前缀
- 错误码先在 `docs/ERROR-CODES.md` 落档，再在 `wb-domain/src/error.rs` 声明常量与文案
- 每个文件 ≤500 行、连续注释 ≤3 行、struct 字段必带注释
- HTTP 只 GET/POST；删除走 `POST /xxx/delete/:id`
- 色值只在 `themes.css`；文案只在 i18n；图标只用 `AppIcon`
- 子进程只经 `wb-tools::exec`（或各自出口自觉对齐同一纪律），探测程序用
  `wb_pkg::proc::resolve_command`，禁止跑 `where` / `which`
