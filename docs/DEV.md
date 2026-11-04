# 开发 · DEV

## 环境

| 依赖 | 用途 |
|---|---|
| Node.js ≥ 22（本机任意安装方式） | 跑构建 / 测试 / 开发脚本 |
| npm | 依赖安装（根 + frontend 各一套） |
| Git | 拉代码与提交（构建产物自带运行时，与开发机无关） |

不需要预装 Python / Rust / 任何编译链——better-sqlite3 与 electron 都走预编译包。

## 命令

```bash
npm install              # 根依赖
cd frontend && npm install && cd ..

npm run dev              # 开发：构建主进程 → Vite 热更 → Electron（WB_DEV=1）
npm run typecheck        # 主进程 TS 严格检查
npm run build            # 主进程 esbuild + 前端 vue-tsc/vite 产物
npx vitest run           # 测试（仓库根 tests/）
npm run dist             # electron-builder 打 NSIS 安装包（输出 release/）
```

纯浏览器调试前端（不拉起 Electron）：主进程跑起来后把端口与令牌写进
localStorage：`wb.dev.port` / `wb.dev.token`，前端会绕过桥直连后端。

## 数据目录

开发态数据在 `%LOCALAPPDATA%\WorkBaby-dev`，打包态在 `%LOCALAPPDATA%\WorkBaby`，
互不污染。想从零开始验证首启链路：删掉对应目录再启动即可（运行时会重新解压，
pi 需要到「应用」页重新安装）。

## 测试策略

测试只保留「改这里会坏那里」的链路，全部在 `tests/`：

| 文件 | 守什么 |
|---|---|
| `e2e_server.test.ts` | 端到端长链路：mock electron → 起 HTTP → 鉴权 → 路由 → service → SQLite → 信封 |
| `session_replay.test.ts` | 会话回放管线：树分支、轮次分组、工具结果挂接、隐藏、压缩、目录编码 |
| `settings_repo.test.ts` | 设置持久化：写 → 关库 → 重开 → 原样读回 |

约定：electron 依赖模块不直接进测试；可测纯逻辑抽到 `pkg/`；
断言说清「断言的是什么」；测不了的路径（真实引擎 / 网络）不写悬空用例。

## 门禁（提交前必过）

```bash
npm run typecheck && npx vitest run && npm run build
```

与 CI 同一套（`.github/workflows/ci.yml`）。只动前端跑 build 即可
（内含 vue-tsc）；只动后端跑 typecheck + vitest。

## 代码纪律速查

- 文件 ≤ 500 行；注释连续 ≤ 3 行，只写「为什么」
- 字段带业务前缀；JSON snake_case；时间戳一律毫秒 number
- 只用 GET / POST；信封恒 200；业务码是数字字符串
- 子进程：argv 直传、windowsHide、超时杀，唯一出口 `pkg/proc`
- 色值只在 `themes.css`，组件只写 `var(--wb-*)`；按钮五态齐全
- 完整规范见 `AGENTS.md`
