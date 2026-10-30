# Agent 内核 · KERNEL

内核负责「让模型持续干活」这件事：管理会话树、驱动采样→工具→回灌循环、
维护转向与追问队列、做自动重试与上下文压缩。

它不感知技能、功能、MCP 与界面的存在——只消费工具列表与系统提示词段，
只产出自己的事件枚举。

## 会话模型

- 会话 = 一个 JSONL 文件 + 内存树
- **上下文 = leaf→root 回放**：每个新条目是当前 leaf 的子；改 leaf 指针即切分支
- 回放产物分两层：系统提示词（命名段，可增量 patch）+ 消息序列

条目类型与存储格式见 `docs/DATA-MODEL.md`。

## 事件枚举

内核唯一出参语言，不含任何界面词汇。界面时序依赖事件顺序——改动顺序等于改界面行为。

| 事件 | 时机 |
|---|---|
| AgentStart | 每次 prompt / continue 开始 |
| TurnStart | 每次 provider 请求前 |
| MessageStart / MessageEnd | 消息开始 / 定稿（定稿即落盘） |
| MessageUpdate(delta) | 流式期间（text / thinking / toolcall 三类 delta，带累积快照） |
| ToolExecutionStart / Update / End | 工具批内（end 带 result / is_error / duration） |
| TurnEnd | assistant 消息 + 全部 toolResult 就绪 |
| QueueUpdate | steering / followUp 队列变化 |
| CompactionStart / End | 上下文压缩前后 |
| AutoRetry | 失败重试（带退避时长） |
| AgentEnd | 循环终点 |
| Settled(aborted) | 完全结算（界面 done 挂这里） |
| SessionInfoChanged / ThinkingChanged / EntryAppended | 状态同步 |

## 主循环

```text
prompt(用户输入)
→ 落 user 条目 → emit AgentStart
→ loop:
    取 steering 队列（one-at-a-time）→ 有则落条目
    emit TurnStart → 组请求（系统提示词段 + 回放上下文 + 工具 schema）
    provider.stream() → 逐 delta emit MessageUpdate
    定稿 assistant 条目（含 usage / stop_reason）
    stopReason:
      toolUse → 校验参数 → emit ToolExecutionStart×n → 并发执行
                （update/end 按完成序，结果按源序回灌）
                → emit TurnEnd → continue
      length → 整批 toolCall 判失败（防截断参数）→ 走重试
      stop   → 无 steering/followup 时跳出
→ emit AgentEnd → Settled
```

**工具批并发**：一批工具同时跑，但**结果按源序回灌**——模型看到的顺序稳定，
不会因为谁先跑完而变化。事件也按完成序发，界面能看到实时进度。

**转向（steering）与追问（followup）**：用户在助手跑的时候插话或追加要求。
转向每轮循环开头取一条（打断当前轮但不丢弃上下文）；追问在一轮结束后处理。
两者都不打断正在进行的工具调用。

## 上下文压缩

窗口快满时自动压缩，避免请求被服务端拒绝。

| 参数 | 值 | 含义 |
|---|---|---|
| reserve | 16384 | 预留给「下一轮生成 + 工具结果」的空间 |
| keepRecent | 20000 | 压缩后保留的最近消息 token 量 |

**切点算法**：在保留窗口内向前找一条不在工具批中间的条目作为切点——
切在工具批中间会丢掉「调用了什么工具」与「工具返回了什么」的配对，
下一轮模型会看到悬空的工具结果。

**清洗**：压缩前先做确定性清洗（去重复的系统提醒、去掉孤立的 toolResult），
再让模型基于清洗后的内容生成摘要。

**压缩记录落盘**：作为 `compaction` 条目写进会话文件，回放时以摘要形态进入上下文——
重启后上下文仍然连贯。

## 自动重试

`agent_end` 后若检测到可恢复错误（网络抖动、上游 5xx、限流），执行：

1. 剔除失败那一批工具的结果（留着会让下一轮重复同样的错误）
2. 指数退避后重试
3. 退避时长与尝试次数发 `AutoRetry` 事件，界面显示「正在重试」

不可重试错误（鉴权失败、参数非法）直接失败，不做无谓重试。

## 子代理

子代理在独立会话文件与独立上下文中运行，用途是把「一次要扫 47 个文件」的活
从主上下文里摘出去——主上下文只看到子代理的结论，看不到中间过程。

工具位与错误码（9704）已预留。

## 约束与取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 树形条目而非线性数组 | 分支与重试剔除需要不可变历史 | 回放要走指针 |
| 确定性清洗 + 预算切点，不靠模型选 | 可测试、可复现 | 摘要质量依赖提示词打磨 |
| 无审批闸门 | 个人单机助手，少一步流程 | bash 直跑有风险；演进位是 7xxx 段加审批事件 |
| usage 信任服务端 | 估算只喂压缩决策 | 个别 provider 无 usage 时用估算兜底（不落库） |
| 结果按源序回灌而非完成序 | 模型看到的顺序稳定 | 慢工具会拖住整批回灌 |
