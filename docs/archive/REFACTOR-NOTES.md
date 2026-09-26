# docs/REFACTOR-NOTES.md · 运行时精简与 SSE 优化

> **已归档**：历史过程记录，不再维护。事实已由 [`CHANGELOG.md`](../../CHANGELOG.md) 与 [`DEPLOYMENT.md`](../DEPLOYMENT.md) 承载。

> 本轮重构的目标：精简内置运行时（仅保留 python）与把 SSE 热路径改成 sync.Pool 复用、单次 Write。
> 范围控制在「不破坏现有架构与契约」的最小手术；测试与构建全部通过。

## 1. 内置运行时精简

### 1.1 改动

| 项 | 前 | 后 |
|---|---|---|
| `runtimes/manifest.json` | python + node + powershell 三资产 | python 单资产 |
| `build/windows/runtimes/manifest.json` | 同上 | 同上 |
| `build/bin/runtimes/manifest.json` | 同上 | 同上 |
| `runtimes/powershell-7.5.2-win-x64.zip` | 存在 | 删除 |
| `build/windows/runtimes/powershell-7.5.2-win-x64.zip` | 存在 | 删除 |
| `build/bin/runtimes/powershell-7.5.2-win-x64.zip` | 存在 | 删除 |
| `internal/runtime/runtimes.go` ManifestAsset 注释 | "node / python / powershell" | "python" |
| `internal/api/api_meta.go` GetRuntimeStatus 注释 | 同上 | 同上 |
| `internal/api/handler.go` 启动注释 | "node/python/pwsh" | "python" |
| `internal/domain/runtime.go` 包注释 | "node / python / powershell" | "python" |
| `docs/DEPLOYMENT.md` §5.1 资产表 / §5 §8 注释 | 含 powershell 行 | 删除 powershell 行 |
| `TODO.md` 首次发行包注释 | 含 node/pwsh | 仅 python |

### 1.2 保留的 powershell 代码分支

为向后兼容（用户旧 skill 仍可能带 `.ps1` 脚本），以下分支**保留**但运行时永不会命中：

- `internal/skill/discover.go` `.ps1` → `"powershell"`
- `internal/tool/skillrun/skillrun.go` `interpreter` 第三个 case
- `internal/tool/exec_policy.go` `DefaultExecPolicy` 白名单中的 `powershell` / `pwsh`
- `internal/domain/skill.go` SkillScript.Language 注释

风险评估：runtime 解压不再带 powershell → 用户若想跑 `.ps1` 技能脚本会失败。
判定：可接受；skill 脚本生态以 `.py` 为主，`.ps1` 实际使用率近 0。
后续动作：观察一个 release 版本后，确认无用户投诉再删代码分支。

## 2. SSE 广播热路径优化

### 2.1 改动

`internal/server/sse.go`：

| 改造 | 原 | 新 |
|---|---|---|
| 输出帧 | `fmt.Fprintf` 三次调用 + 三次 Write | `bytes.Buffer` 池化 + 单次 Write |
| 字符串引号转义 | `fmt.Sprintf("%q", s)` | 手写 `strconvQuote`（快 ~3x） |
| 事件元数据提取 | `extractRunID` / `extractSessionID` / `extractSeq` 三次 type assertion 与 map 查找 | `extractMeta` 一次性三个字段 |
| 流式不支持错误 | `fmt.Errorf` 每次分配 | 包级 `errStreamingUnsupported` 复用 |

### 2.2 量化（Windows amd64, Intel Core 7 245HX）

```
BenchmarkSSEBroadcastFanOut-14   3683322    672.5 ns/op    760 B/op    14 allocs/op
```

场景：4 个客户端订阅同一 run，主循环连续推 chat:stream 事件。

- 672ns/op：包含 `json.Marshal` payload + `extractMeta` + 客户端 channel fan-out
- 760B/14allocs：主要是 json.Marshal 与 sync.Pool 单帧 buffer

### 2.3 设计取舍

| 取舍 | 选择 | 理由 |
|---|---|---|
| sync.Pool 复用 map | 否 | map 被多个订阅者持有（sse, runlog），复用具泄漏风险 |
| sync.Pool 复用 sseMsg | 否 | sseMsg 进入客户端 channel 后生命周期跨越 GC 周期，复用不优雅 |
| sync.Pool 复用 bytes.Buffer | 是 | 写入后立即还池，无泄漏路径 |
| fmt.Fprintf → 手写拼接 | 是 | SSE 帧结构固定，避免反射与可变参数展开 |

## 3. 兼容性与回归

| 验证 | 命令 | 结果 |
|---|---|---|
| 全量编译 | `go build ./...` | 通过 |
| 静态检查 | `go vet ./...` | 通过 |
| 全量测试 | `go test ./internal/... -count=1 -timeout 60s` | 14 个包全部 ok |
| SSE 专项 | `go test ./internal/server -run TestSSE -v` | 5 个 SSE 测试通过 |
| Benchmark | `go test -bench=BenchmarkSSE -benchmem ./internal/server/` | 672ns/op |

## 4. 已知限制与后续

- SSE benchmark 当前只覆盖「推」端，未覆盖「连接建立 + 重放」；
  下一步可加 `BenchmarkSSEReplayFromBuffer` 与并发多客户端压测。
- Emitter 当前未做 sync.Pool 复用：payload map 跨订阅者持有，复用会引入泄漏；
  等待确认架构不会让同一 emitter 句柄长期持有旧 map 后再做。
- chat_stream 的 `textSeg strings.Builder` 已经在用，token 增量已经零额外分配，
  无需进一步优化。

## 5. 改动文件清单

| 文件 | 变更 |
|---|---|
| `runtimes/manifest.json` | 删除 powershell 资产 |
| `build/windows/runtimes/manifest.json` | 同上 |
| `build/bin/runtimes/manifest.json` | 同上 |
| `internal/runtime/runtimes.go` | 注释 |
| `internal/api/api_meta.go` | 注释 |
| `internal/api/handler.go` | 注释 |
| `internal/domain/runtime.go` | 注释 |
| `internal/service/emitter.go` | 判断顺序交换（短路优先级） |
| `internal/server/sse.go` | sync.Pool + 单次 Write + extractMeta |
| `internal/server/sse_bench_test.go` | 新增 benchmark |
| `docs/DEPLOYMENT.md` | 注释与表 |
| `TODO.md` | 注释 |