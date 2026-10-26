# 06 · 模型、提示与人设

## 模型配置只有一份

`~/.pi/agent/models.json` 是服务商与模型的唯一真相：

```json
{
  "providers": {
    "my-gateway": {
      "name": "我的网关",
      "baseUrl": "https://api.example.com/v1",
      "api": "openai-completions",
      "apiKey": "sk-…",
      "models": [
        { "id": "m1", "name": "模型一", "contextWindow": 128000, "maxTokens": 8192,
          "reasoning": true, "input": ["text", "image"] }
      ]
    }
  }
}
```

WorkBaby 的写入口只改自己掌管的字段（`name` / `api` / `baseUrl` / `apiKey` / `models`），
服务商里的未知字段（`headers` / `modelOverrides` / 未来的兼容开关）原样保留。

| 规则 | 原因 |
|---|---|
| 空密钥表示「不改」 | 界面上是掩码，用户没动它就不该把已存密钥清掉 |
| 界面只显示掩码 | 明文只在用户点「显示」时回传一次 |
| 写完后重启空闲进程 | 配置是启动期读的，不重启等于没改 |

## 默认模型

`settings.json` 的 `defaultProvider` / `defaultModel`：

- 「设默认」写它；
- 界面显示用的是 `EffectiveDefaults()`：没设时取第一个有模型的服务——
  PI 自己也是这么自动选的，界面不能显示「没配模型」而去逼用户点一次。

## 思考档

三处出现，含义一致（`off` / `minimal` / `low` / `medium` / `high` / `xhigh` / `max`）：

| 位置 | 作用域 | 写入 |
|---|---|---|
| 输入框的思考档 chip | 当前会话 | `set_thinking_level`（写进会话文件） |
| 设置 · 助手引擎 →「运行参数」的思考档 | 全局默认 | `settings.json` 的 `defaultThinkingLevel` |
| 模型条目的 `thinkingLevelMap` | 该模型支持哪些档 | `models.json` |

界面按当前模型 `thinkingLevelMap` 里非空的档位列出可选项（`ProviderService` 把它读成
`thinking_levels[]` 随模型列表下发）；模型不支持的档 PI 自己会钳制。
没配 `thinkingLevelMap` 的模型退化为完整七档。

## 压缩参数

`settings.json` 的 `compaction`：

| 字段 | 缺省 | 含义 |
|---|---|---|
| `reserveTokens` | 由 PI 定 | 保留给输出的余量，低于它开始整理 |
| `keepRecentTokens` | 由 PI 定 | 最近这些 token 不进摘要 |

界面只暴露这两项；`enabled` 等 PI 自己的字段按字段级合并保留。

**压缩本身由 PI 做**：确定性清洗 + 按预算找切点。WorkBaby 只展示 `compaction_end` 的结果。

## 人设：不写用户的文件

WorkBaby 的说话方式（不用 emoji、不客套、直接报结果）以
`--append-system-prompt <数据目录>/persona.md` 的形式传给 PI 子进程。

**不写 `APPEND_SYSTEM.md`**：那是用户的文件，改了会让他在终端里跑 `pi` 时也吃到 WorkBaby 的人设。
每次启动重写 `persona.md`，它跟着版本走。

## 提示模板与斜杠命令

`/` 菜单上三类东西来自三个不同的地方，这个分别必须讲清楚：

| 种类 | 来源 | 怎么生效 |
|---|---|---|
| 扩展命令 | `get_commands` 的 `source=extension` | 把 `/名字 ` 填回输入框，发送后由 PI 展开 |
| 提示模板与技能 | `get_commands` 的 `source=prompt` / `skill`（技能名带 `skill:` 前缀） | 同上：发文本，PI 自己把模板与正文展开 |
| 内置命令 | **不在 `get_commands` 里** | 界面自己执行，映射到对应端点 / 路由 |

`get_commands` 只报扩展、模板与技能三类：PI 那一组内置命令（`compact` / `model` / `thinking` /
`new` / `export` / `copy` / `name` / `session` / `reload` / `login` …）是它的终端界面自己解析
输入文本实现的，RPC 模式下不会报上来，把 `/compact` 当正文发出去只会被当成一句话。

所以菜单里那十项内置命令是界面的动作项，名字与 PI 对齐、干活走我们自己的端点：
`/new` → 新建会话、`/model` `/thinking` → 开对应 chip 的弹层、`/compact` → `POST /chat/compact`（PI 的 `compact`）、
`/session` → 会话信息、`/export` → `POST /sessions/export/:id`、`/copy` → 复制本轮、
`/reload` → 重扫技能与命令、`/login` `/settings` → 路由。
只活在终端里、GUI 没有对应动作的（`/tree` `/share` `/bug` `/hotkeys` `/quit`）不列入：
列一条执行不了的命令比不列更坏——它看起来是个入口，点下去什么也不会发生。

命令清单不在本地拼盘：扩展注册的命令只有进程知道，磁盘扫描看不到「这个模板是否被包过滤掉」。

## 取舍

- **models.json 直连**：与 `pi` / 其他配置工具共用一份；代价是明文密钥落在文件里（PI 本来就是这么存的）。
- **人设走命令行参数**：不污染用户配置；代价是每次改人设要重写那个文件（启动时做）。
- **界面只暴露两个压缩参数**：步数上限、工具并发这类内核参数没有协议面可写，摆一个框就是假开关。
