# 02 · 上下文压缩

## 定位

长会话超出模型窗口时的兜底：**确定性**清洗与裁剪，**不调 LLM**。
零延迟、零成本、可单测——这是与「用模型写摘要」路线的核心取舍：
个人助手的会话大多在几十轮以内，为偶发的超长会话付一次模型调用的钱和延迟不划算。

## 设计

### 流水线

```text
msgs → 估算 token → 超预算？
                    ├─ 否 → 原样发送（不做任何改动）
                    └─ 是 → CleanForProtocol → FindCutPoint 切 → 裁掉前段 → CleanForProtocol 复检
```

先估后清：没超预算就不动一个字节。清洗只在「已经要裁」时才做，
避免每次发送都重排消息（省掉无谓的内存分配与语义变化）。

### CleanForProtocol（协议硬约束）

发送上游前必须通过，否则下一轮直接 400：

| 输入形态 | 处理 |
|---|---|
| 相邻的 assistant 消息（同一轮的并行声明被拆散） | 合并成一条，`tool_calls` 取并集 |
| 空 assistant（无内容无思考无调用） | 删除 |
| tool 结果找不到对应调用（孤儿） | 删除 |
| assistant 声明了调用但没有结果 | 补一条 `IsError` 的 tool 占位 |

**为什么必须先合并**：同轮多个工具声明若被逐条落库，链上会留下
`A(c1) → A(c2) → T(c1) → T(c2)`——`T(c1)` 与它的声明之间隔着 `A(c2)`，
上游以 `tool result's tool id not found` 直接拒绝。相邻 assistant 本就是
同一轮的产物，合并即恢复配对。

### EstimateTokens

一条请求的实际占用由四部分组成，少算任何一项判断就会偏乐观（水位读低、压缩来晚）：

| 组成 | 折算 | 在哪算 |
|---|---|---|
| system 提示词 | 同正文口径，另加一条消息的结构开销 | `service.budget` → `Budget.SystemTokens` |
| 消息（正文 / 思考 / 工具调用声明） | 中文按字（每个 Han 字符 1 token）、其余按 3.5 字符 1 token；每条消息 4 token 结构开销，每个工具调用 8 token | `EstimateTokens` |
| 图片 | 每张 1200 token 的固定折算：识图模型的图片开销由上游缩放策略决定，本地只能给量级正确的值。不折算时「粘贴一张截图」的那一轮会被严重低估 | `EstimateTokens` |
| 工具声明（schema） | 每个工具的名字 + 描述 + schema JSON | 内核 `New` → `Budget.ToolsTokens` |

3.5 而不是 4：代码与 JSON 的 token 密度高于自然语言，按 4 估会系统性偏低，偏低的估算
让压缩判断偏乐观，最终以上游 context 超限报错收场。
误差 ±10% 对「要不要裁」这个判断足够，不值得引入 tokenizer 依赖。

`service.budget` 的 `SystemTokens` 同样走 `agent.EstimateTokens`：
system 提示词里中文占比高，另写一套 `len/4` 会低估约 1/4。

### Budget（阈值来自 service 层）

```go
type Budget struct{ Window, WindowKnown, Reserve, Keep, SystemTokens, ToolsTokens }
```

| 字段 | 缺省 | 含义 |
|---|---|---|
| Window | 128000 | 模型上下文窗口：来自内置能力目录，可被模型级配置与全局 `context_window` 设置覆写；目录认不出的模型用 `DefaultContextWindow`(=128000)——偏小的代价是压缩早一点（可恢复，读数还会加「约」前缀），偏大的代价是首轮撞上游窗口上限 400（整轮作废） |
| WindowKnown | — | 窗口是否为确切值；false 时界面读数加「约」前缀 |
| Reserve | 跟随输出预算 | 给模型输出留的余量：缺省 = 该模型真正下发的输出预算，用户设置（`context_reserve_tokens`）只作为显式覆写 |
| Keep | 20000 | 裁剪后保留的近期 token 预算；随窗口缩放 = `clamp(窗口/4, 20000, 200000)` |
| SystemTokens | 估算 | system 提示词的 token 估算，判断是否超预算必须算上它 |
| ToolsTokens | 内核自动填 | 工具声明的 token 估算，由 `New` 按 `Config.Tools` 数好写回：schema 每轮都随请求发出去，和消息一样占窗口，调用方少填一项压缩就会偏晚 |

`fullContext(b, msgs) = SystemTokens + ToolsTokens + EstimateTokens("", msgs)` 是唯一口径：
水位读数、压缩事件、压缩判断三处都用它，读数之间才不会互相打架。

输出预算（`MaxTokens`）= **窗口 1/8**（`ModelCapability.OutputBudget`）：保证长回答不被
随手截断。窗口来自内置目录时再与厂商硬上限取小——1M 窗口按 1/8 会算出 12.5 万，
而有的模型的单次输出上限只有 32K / 64K，越界就是整轮 400。

窗口由用户手填（`model_configs` 表或全局 `context_window`，`WindowOverride=true`）时
硬上限不参与：目录里的硬上限描述的是同名模型在另一个服务上的样子（网关转发的模型、
私有部署改名的模型都可能对不上），此时按用户声明的窗口 1/8 走，被拒了有下面的降级重试兜底。

可用预算 = `Window - Reserve`。Reserve 缺省就是输出预算本身：留的余量比实际输出
预算还小，等于按虚高的空间往窗口里塞内容，总占用会顶破窗口。余量本身超过整个
窗口时（极端配置）压到 `Window/4`，否则预算恒为负、压缩会退化成什么都不裁；
内核另留 `Window/2` 兜底，用于 Budget 由其他调用方构造的场景。

### 上游拒绝时的降级重试

本地估算的两个数（上下文占用、输出预算）都可能比上游的真实上限大，上游会拒绝整轮。
内核按拒绝的类别降级重试——同一轮里合计不超过两次（`maxRetryTurns`）——
两条路径都不改历史，只是换了参数或换了上下文再发：

| 上游错误 | 识别 | 降级动作 |
|---|---|---|
| `context length exceeded` 一类 | `llm.IsContextOverflow` | `CompactForce`：保留量砍半、允许从中间硬切，再发一次（同一轮最多压缩一次） |
| `Invalid max_tokens / max_completion_tokens` 一类 | `llm.IsOutputLimit` | `llm.OutputLimitFrom(err, 当前预算)` 取新预算再发：报错里写着数字就用那个数（`[1, 32768]` / `128000 > 32768` 都出现过），没写就对折；收紧到 4096 以下就放弃——再小短到没用，把上游原话交给用户 |

约束：本轮**已经吐出过内容**（正文 / 思考 / 工具调用）就不重试——重试会让用户
看到一段重复的正文。全部降级用完仍失败才以错误收尾。这条路径保证「长任务跑到一半
突然不动了」变成「它整理了一下上下文 / 收了一下输出预算，继续跑」。

输出预算只降不升：内核不知道模型能吐多少，抬过头就是把「能看懂的截断」换成
「看不懂的 400」（见 spec 01 的截断策略）。降级重试本身很便宜——被拒的轮次
一个 token 都没产出，但次数必须有上限，否则遇到恒拒绝的端点会一直试下去。

### FindCutPoint（切点选择）

从尾部按 token 预算向前累加，在**最近的 user 消息**处切——
保证留下的第一句是用户的话，不出现「以工具结果开头」的畸形上下文。

切点 ≤ 1（一个完整 turn 都留不下）时放弃裁剪：宁可让上游报错，
也不要发一段残缺上下文过去。

### Compact（内核入口）

```go
func Compact(msgs []llm.Message, b Budget) (out []llm.Message, after int)
```

返回裁剪后的消息与裁剪后 token（裁剪前由调用方按消息条数对比得出）。
**只有真的裁掉东西才发 `compressed` 事件**——每次都发会让前端不停闪「上下文已整理」。

## 契约

```go
func CleanForProtocol(msgs []llm.Message) []llm.Message
func EstimateTokens(system string, msgs []llm.Message) int
func FindCutPoint(msgs []llm.Message, keepTokens int) int
func Compact(msgs []llm.Message, b Budget) (out []llm.Message, after int)
func CompactForce(msgs []llm.Message, b Budget) (out []llm.Message, after int)
```

## 取舍

不落库压缩记录。数据库永远存完整历史，压缩只发生在「发给模型的那一刻」——
重开会话会重新裁一次，结果完全一致。会话详情因此永远是完整的，
不会出现「上周的对话从中间开始」这种让新手困惑的界面。
代价是超长会话每次发送都要重算一遍 token 估算，好在这是纯内存计算，
几十轮量级的会话耗时可以忽略。

## 测试

`backend/agent/agent_test.go`：

| 测试 | 锁住的行为 |
|---|---|
| `TestCompactProtocol` | 清洗硬约束（合并拆散的并行声明、剔除空 assistant / 孤儿结果）；压缩永不孤儿化工具结果（扫全切点）；整轮丢弃；预算边界与降级截断 |
| `TestLoopChain`（输出预算被拒子测试） | 上游拒绝输出预算时按报错里的上限降级重试（没写数字才对折，最多两次、下限 4096），首次下发的值不被改动 |
