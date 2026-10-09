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

中文按字（每个 Han 字符 1 token）、其余按 3.5 字符 1 token，再加每条消息 4 token
结构开销与每个工具调用 8 token（声明的名字与固定字段）。3.5 而不是 4：代码与 JSON
的 token 密度高于自然语言，按 4 估会系统性偏低，偏低的估算让压缩判断偏乐观，
最终以上游 context 超限报错收场。
误差 ±10% 对「要不要裁」这个判断足够，不值得引入 tokenizer 依赖。

`service.budget` 的 `SystemTokens` 同样走 `agent.EstimateTokens`：
system 提示词里中文占比高，另写一套 `len/4` 会低估约 1/4。

### Budget（阈值来自 service 层）

```go
type Budget struct{ Window, WindowKnown, Reserve, Keep, SystemTokens }
```

| 字段 | 缺省 | 含义 |
|---|---|---|
| Window | 65536 | 模型上下文窗口：来自内置能力目录，可被模型级配置与全局 `context_window` 设置覆写；认不出的模型用保守值——偏小的代价是压缩早一点（可恢复），偏大的代价是首轮 400（整轮作废） |
| WindowKnown | — | 窗口是否为确切值；false 时界面读数加「约」前缀 |
| Reserve | 跟随输出预算 | 给模型输出留的余量：缺省 = 该模型真正下发的输出预算，用户设置（`context_reserve_tokens`）只作为显式覆写 |
| Keep | 20000 | 裁剪后保留的近期 token 预算；随窗口缩放 = `clamp(窗口/4, 20000, 200000)` |
| SystemTokens | 估算 | system 提示词的 token 估算，判断是否超预算必须算上它 |

输出预算（`MaxTokens`）= **窗口 1/8 与厂商硬上限取小**（`ModelCapability.OutputBudget`）：
窗口 1/8 保证长回答不被随手截断，硬上限保证不越界——1M 窗口按 1/8 会算出 12.5 万，
而有的模型的单次输出上限只有 32K / 64K，越界就是整轮 400。

可用预算 = `Window - Reserve`。Reserve 缺省就是输出预算本身：留的余量比实际输出
预算还小，等于按虚高的空间往窗口里塞内容，总占用会顶破窗口。余量本身超过整个
窗口时（极端配置）压到 `Window/4`，否则预算恒为负、压缩会退化成什么都不裁；
内核另留 `Window/2` 兜底，用于 Budget 由其他调用方构造的场景。

### 上游报超限时的强制压缩

本地估算是粗估，偏乐观时上游会以 `context length exceeded` 一类错误拒绝整轮请求。
内核识别这类错误（`llm.IsContextOverflow`）后走 `CompactForce`：
保留量砍半、允许从中间硬切，压缩后重试本轮**一次**。

约束：本轮**已经吐出过内容**（正文 / 思考 / 工具调用）就不重试——重试会让用户
看到一段重复的正文。第二次仍失败才以错误收尾。这条路径保证「长任务跑到一半
突然不动了」变成「它整理了一下上下文，继续跑」。

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
func TruncateDeterministic(msgs []llm.Message, keep int) []llm.Message
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

`backend/agent/agent_test.go`（`TestCompactProtocol`）：

| 测试 | 锁住的行为 |
|---|---|
| `TestCompactProtocol` | 清洗硬约束（合并拆散的并行声明、剔除空 assistant / 孤儿结果）；压缩永不孤儿化工具结果（扫全切点）；整轮丢弃；预算边界与降级截断 |
