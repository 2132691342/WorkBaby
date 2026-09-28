# 02 · 上下文压缩

## 定位

长会话超出模型窗口时的兜底：**确定性**清洗与裁剪，**不调 LLM**。
零延迟、零成本、可单测——这是与「用模型写摘要」路线的核心取舍：
个人助手的会话大多在几十轮以内，为偶发的超长会话付一次模型调用的钱和延迟不划算。

## 设计

### 四步流水线

```text
msgs → CleanForProtocol → 估算 token → 超预算？
                                    ├─ 否 → 原样发送
                                    └─ 是 → FindCutPoint 切 → 裁掉前段 → CleanForProtocol 复检
```

### CleanForProtocol（协议硬约束）

发送上游前必须通过，否则下一轮直接 400：

| 输入形态 | 处理 |
|---|---|
| 空 assistant（无内容无思考无调用） | 删除 |
| tool 结果找不到对应调用（孤儿） | 删除 |
| assistant 声明了调用但没有结果 | 补一条 `IsError` 的 tool 占位 |

### EstimateTokens

中文按字（每个 Han 字符 1 token）、其余按 4 字符 1 token，再加每条消息 4 token
结构开销。误差 ±10% 对「要不要裁」这个判断足够，不值得引入 tokenizer 依赖。

### Budget（阈值来自 service 层）

```go
type Budget struct{ Window, Reserve, Keep int }
```

| 字段 | 缺省 | 含义 |
|---|---|---|
| Window | 128000 | 模型上下文窗口，按模型名粗判，认不出用缺省 |
| Reserve | 16384 | 给模型输出留的余量，可由设置项覆盖 |
| Keep | 20000 | 裁剪后保留的近期 token 预算 |

可用预算 = `Window - Reserve`；窗口比余量还小时退化为 `Window / 2`。

### FindCutPoint（切点选择）

从尾部按 token 预算向前累加，在**最近的 user 消息**处切——
保证留下的第一句是用户的话，不出现「以工具结果开头」的畸形上下文。

切点 ≤ 1（一个完整 turn 都留不下）时放弃裁剪：宁可让上游报错，
也不要发一段残缺上下文过去。

### Compact（内核入口）

```go
func Compact(msgs []llm.Message, b Budget) ([]llm.Message, int, int)
```

返回裁剪后的消息、裁剪前 token、裁剪后 token。**只有真的裁掉东西才发
`compressed` 事件**——每次都发会让前端不停闪「上下文已整理」。

## 契约

```go
func CleanForProtocol(msgs []llm.Message) []llm.Message
func EstimateTokens(system string, msgs []llm.Message) int
func FindCutPoint(msgs []llm.Message, keepTokens int) int
func TruncateDeterministic(msgs []llm.Message, keep int) []llm.Message
func Compact(msgs []llm.Message, b Budget) (out []llm.Message, before, after int)
```

## 取舍

不落库压缩记录。数据库永远存完整历史，压缩只发生在「发给模型的那一刻」——
重开会话会重新裁一次，结果完全一致。会话详情因此永远是完整的，
不会出现「上周的对话从中间开始」这种让新手困惑的界面。

## 测试

`internal/agent/compact_test.go`：

| 测试 | 锁住的行为 |
|---|---|
| `TestCleanForProtocolHardConstraints` | 剔除空 assistant 与孤儿结果；补齐未配对调用 |
| `TestFindCutPointPrefersUserBoundary` | 切点只落在 user 边界 |
| `TestTruncateDeterministicKeepsRecentTurns` | 降级截断保留完整 turn |
| `TestCompactBudgetPolicy` | 未超预算不动；切点不足原样返回 |
