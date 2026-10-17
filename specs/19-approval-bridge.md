# 19 · 审批与 WorkBaby 扩展

PI 默认**不**在每次工具调用前问用户。WorkBaby 要有审批闸门，唯一不改 PI 的接法是
给它装一个扩展：`tool_call` 处理器阻塞等待 `ctx.ui.confirm()`，
而 RPC 模式下的 `confirm` 恰好是一条 `extension_ui_request` / `extension_ui_response` 对话对。

于是审批链路是这样：

```
PI 的 tool_call 处理器 → ctx.ui.confirm(标题, 详情)
   → stdout: {"type":"extension_ui_request","id":..,"method":"confirm",...}
   → WorkBaby 翻译成 chat:approval 事件 → SSE → 用户点「允许 / 拒绝」
   → stdin: {"type":"extension_ui_response","id":..,"confirmed":true}
   → 扩展 return {block:true} 或放行
```

## 1. 谁装这个扩展

WorkBaby 在启动时把扩展源码**写进** `<agent-dir>/extensions/workbaby.ts`，
并在 `settings.json` 的 `extensions` 数组里登记它。用户不需要 npm、不需要编译
（PI 用 jiti 直接跑 TypeScript）。

一个文件装两件事：**审批闸门**与**记忆写入工具**。它们同属"WorkBaby 对 PI 的控制面"，
拆成两个文件只会让"WorkBaby 到底改了 PI 什么"更难回答。

## 2. 策略从哪来

扩展在 `session_start` 读一次 `<agent-dir>/workbaby-policy.json`：

```json
{
  "permission": "ask_write",
  "timeoutMs": 1800000
}
```

| `permission` | 行为 |
|---|---|
| `auto` | 全部放行，不弹任何确认 |
| `ask_write`（默认） | 非只读工具都确认（写文件、跑命令、装包、unknown） |
| `readonly` | 只放行只读工具，其余一律拒绝 |

判定依据用 PI 的工具注解（`readOnlyHint` / `destructiveHint` / `openWorldHint`）——
那是 MCP 的标准语义，PI 的内置工具与 MCP 工具都带。

**`readonly` 是真的拒绝**，不是弹窗等用户点允许：这个档位的语义是"只看不改"，
让它弹出可批准就等于没这个档位。

## 3. 超时

`confirm` 请求带 `timeout`（默认 30 分钟）。超时由 PI 侧自动以 `false` 收口，
WorkBaby 不需要自己记时。这与旧设计里"30 分钟无人值守按拒绝"一致。

**为什么会话关掉不会卡住**：用户关掉窗口时 WorkBaby 发 `abort`，
PI 收尾时那些等待中的 `confirm` 以 `cancelled` 收口。残留的审批记录在下次启动
被扫成「已过期」并在界面上提示一次。

## 4. 事件映射

| PI 记录 | WorkBaby |
|---|---|
| `extension_ui_request` / `method=confirm` | `chat:approval`（`{approval_id, tool, title, message, risk}`） |
| `extension_ui_request` / `method=select` | `chat:approval`（选项作为 `options[]` 带出去） |
| `extension_ui_request` / `method=notify` | 忽略（写日志） |
| `extension_ui_request` / `method=setStatus` 等 | 忽略 |

**只有 `confirm` 与 `select` 需要回响应**。其余方法不回就是正确行为——
给不需要响应的请求回一条 `extension_ui_response` 会让 PI 的匹配表里积垃圾。

## 5. 决策路径

```
POST /approvals/decide/:id  {decision: "allow" | "deny" | "always"}
  ├─ 找等待表里的 approval_id → 摘走（摘不到 = 4102，已超时/已决策）
  ├─ allow   → 回 extension_ui_response{confirmed:true}
  ├─ deny    → 回 extension_ui_response{confirmed:false} → PI 得到 block
  └─ always  → 写 settings KV（本会话放行该工具）+ 回 confirmed:true
```

`always` 是**会话级**的：写进 `sessions` 的一列（`allowed_tools`），不写全局。
全局放行等于把 `ask_write` 档位废掉，而用户点「本会话总是允许」时不想要这个。

**决策只有一次机会**：从等待表摘走条目的人拿唯一发送权。
摘不到说明决策在途（超时或重复点击），返回 `4102` 且不改任何状态——
否则界面显示「已批准」，PI 那边早已按拒绝跳过。

## 6. 记忆写入工具

同一个扩展注册 `memory_write`：

```typescript
pi.registerTool({
  name: "memory_write",
  description: "把一条跨对话仍然成立的事实或偏好记下来。只记用户明确说过的，不记过程。",
  parameters: { type: "object", properties: { content: { type: "string" } }, required: ["content"] },
  async execute(args) {
    await fetch(`${HOST}/api/v1/memories`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ content: args.content }),
    })
    return { content: [{ type: "text", text: "已记住" }] }
  },
})
```

`HOST` 由 WorkBaby 在生成扩展时写入（继承端口握手拿到的地址）。
扩展跑在 PI 进程里，能读到注入的环境变量。

记忆的**注入**不走工具，走 `<agent-dir>/APPEND_SYSTEM.md` 的受控标记块
（见 [07](07-prompt-and-settings.md)）——注入要每轮自动生效，
让模型自己想起来去查等于没有。

## 7. 为什么不用 PI 的 project trust 代替

`--approve` / `defaultProjectTrust` 管的是「这个文件夹能不能加载可执行扩展」，
是**每次启动一次**的判定，不是每次工具调用。两件事的粒度差三个数量级。

## 8. 不变量

| 不变量 | 锁死原因 |
|---|---|
| 扩展必须在 `settings.json` 的 `extensions` 里登记 | 只写文件不登记，PI 根本不加载它，表现为「审批档设了没用」 |
| 生成扩展时重写整个文件 | 追加会让多次启动累积重复的处理器，同一个调用弹两次确认 |
| 只有 `confirm` / `select` 需要回响应 | 给 fire-and-forget 方法回响应会让 PI 的匹配表积垃圾 |
| `readonly` 直接拒绝而不弹窗 | 弹了可批准就等于没有这个档位 |
| `always` 只作用于会话 | 写全局等于把 `ask_write` 档位废掉 |
| 决策摘不到条目就返回 4102 | 重复点击/超时后到达的决策若改库，界面显示已批准而工具早已按拒绝跳过 |
| 扩展里不存密钥与业务配置 | 扩展文件是明文可读的用户文件；它只能调本机 HTTP，不自己持有秘密 |
| `timeout` 交给 PI 侧 | 两边各记一套超时必然分叉，且 WorkBaby 的重启会丢掉计时 |

## 9. 代价

- **多一个 WorkBaby 自有的扩展文件**：用户如果删掉它，审批闸门静默消失
  （工具照常跑，只是不问）。所以启动时要校验它存在且版本匹配，不对就重写。
- **审批依赖扩展的稳定性**：扩展抛错时 PI 的 `tool_call` 处理器失败会**阻塞工具**（fail-safe），
  表现为「工具一直不执行」。这是可接受的失败方向，但要在日志里说清原因。
- **`readonly` 档会拦掉一些其实无害的调用**（比如工具没声明注解），
  那是刻意的：不知道就别做。
