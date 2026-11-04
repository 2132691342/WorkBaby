# 部署 · DEPLOY

## 打包形态

electron-builder → NSIS 安装包（x64，per-user 安装，可选安装目录）。
产物输出 `release/`（已 gitignore）。

随包分发的资产（`extraResources`，装到 `<安装目录>\resources\assets`）：

| 资产 | 内容 | 作用 |
|---|---|---|
| `assets/runtime/*.zip` | node v24.21 / PortableGit / Python 3.13 | 首启后台解压进数据目录 |
| `assets/skills/` | 内置技能包 | 首启复制进沙箱技能目录 |
| `assets/docs/` | 帮助文档 markdown | 帮助页直读 |

代码产物：主进程 esbuild 打成 `dist/main/index.js`（CJS 单文件，
electron 与 better-sqlite3 保持外部依赖），preload 打成 `dist/preload/index.cjs`，
前端 Vite 产物 `frontend/dist/`。better-sqlite3 的原生模块经 `asarUnpack` 解包。

## 版本规则

- 版本号唯一落点：根 `package.json` 的 `version`。
- 发布 = 打 `v<版本>` tag（如 `v1.0.0`），release workflow 校验
  tag 与 package.json 一致后构建发布；不一致直接失败。
- 数据目录不区分版本；应用升级只换代码与资产，用户数据原地保留
  （运行时已解压则跳过，pi 按需在应用页升级）。

## CI/CD

| workflow | 触发 | 内容 |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR → main | npm ci → typecheck → vitest → build（含前端 vue-tsc） |
| `.github/workflows/release.yml` | push tag `v*` | 版本校验 → npm ci ×2 → build → `electron-builder --win nsis --publish always` |

release 需要 `GITHUB_TOKEN`（workflow 内置注入）创建 Release 并上传安装包。
CI 里设置 `ELECTRON_SKIP_BINARY_DOWNLOAD=1` 跳过 Electron 二进制下载
（构建与测试用不到，打包由 electron-builder 自行拉取）。

## 发布步骤

```bash
# 1. 确认门禁全绿
npm run typecheck && npx vitest run && npm run build

# 2. 确认 package.json version 与目标 tag 一致

# 3. 打 tag 并推送（已存在则强制覆盖）
git tag -f v1.0.0
git push origin main
git push origin -f v1.0.0

# 4. GitHub Actions 的 Release workflow 出安装包并挂到 Release 页
```

## 本地出包验证

```bash
npm run dist        # 产物在 release/，安装后验证：
```

- 首启：窗口就绪 → 运行时后台解压（应用页可见三个内置运行时行）
- 应用页：安装 pi → 版本显示正常
- 模型页：填服务商 Key → 测试连通 → 会话页开聊
- 关窗默认收托盘，托盘可还原 / 退出
