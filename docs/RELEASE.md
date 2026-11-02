# 发布 · RELEASE

发布即把主干推到 GitHub 的 `main` 分支，并把 `v1.0.0` 这个标签强制对齐到当前主干头。

## 门禁先于推送

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

## 发布步骤

1. 把改动整理成一次语义提交（`git add -A && git commit`），提交信息写清做了什么与为什么。
2. 推主干：`git push origin main`。
3. 把 `v1.0.0` 对齐到当前头并强制覆盖（标签代表「当前稳定线」，不是历史快照）：
   `git tag -f v1.0.0 && git push -f origin v1.0.0`。

强制覆盖而不是新建递增标签，是因为本项目把 `v1.0.0` 当作主干稳定线——
任何一次通过门禁的 `main` 头都是合法的 1.0.0。需要保留历史快照时改用别的标签名，不占用 `v1.0.0`。

## 打包

桌面安装包由 Tauri 构建产物产出，流程见 [DEPLOY.md](DEPLOY.md)。
内置运行时（PortableGit 与 Python 归档）编译期内嵌进 `wb-service`，
不进仓库大文件，打包时随二进制一起分发。
