# 权限与审批

WorkBaby 的「能不能干活、谁说了算」由两层机制决定。本篇讲清两者的关系与日常用法。

## 1. 权限四档（模式矩阵）

会话级权限模式控制 AI 的「自动驾驶范围」。会话级可覆盖全局设置。

| 模式 | readonly | write_local | exec / network | destructive |
|---|---|---|---|---|
| `restricted` | allow | ask | ask | ask |
| `default` | allow | ask | ask | ask |
| `auto_edit` | allow | allow | ask | ask |
| `yolo` | allow | allow | allow | allow |

### 1.1 各档含义

- **`restricted`**：与 `default` 等价（两者只差「是否 deny」；deny 让模型无法改道，收益低于代价，故未保留）
- **`default`**：默认档。只读工具（`file_read` / `webfetch` / `doc_reader`）直行；其余（写文件 / 执行命令 / 联网 / 删除）每次问
- **`auto_edit`**：工作区内的写文件（`file_write` / `archive_manager` / `memory_write`）直行；执行命令与联网仍需问
- **`yolo`**：全部放行（不推荐；调试 Skill 时临时切到 `yolo` 验证）

### 1.2 切换位置

- 全局：设置页 → **系统 → 安全与高级 → 默认权限模式**
- 会话级：会话右侧 → **权限** 下拉（覆盖全局）
- 全局+会话级组合的最终值显示在「权限」下拉旁边

## 2. 审批门（人工决策）

当工具命中 `ask` 档，模型会被挂起，等用户决策。**审批是离散的、可记忆的、可跨 run 复用的**。

### 2.1 审批卡片字段

| 字段 | 用途 |
|---|---|
| 命令 / 路径 | 工具将要做的事（来自工具的 `RiskClassifier`，降级为「工具名 + 参数」）|
| 工作目录 | 真实运行目录（`exec` 工具 meta.cwd）|
| 风险等级 | `needs_approval` / `irreversible` / `input_required` |
| 已等待时长 | 卡片挂起的秒数 |

### 2.2 三种决策

| 决策 | 语义 | 持久化 |
|---|---|---|
| **拒绝** | 当前 run 跳过该调用；模型读到拒绝原因可改道 | 否 |
| **本会话允许** | 当前会话后续同命令直接放行 | 是（`approval_grants` 表）|
| **仍要批准** | 此次放行，下次同命令重新问 | 否 |

### 2.3 不可逆操作

风险等级 `irreversible`（如 `rm -rf` / 删表 SQL / 文件覆写）有特殊保护：

- `本会话允许` 按钮**隐藏**（不可一次性免审）
- 每次必须单独批准
- 即便历史 `grants` 表里有同名命令，本次也会重新问

这是「不能撤销的操作必须每次亲手确认」的硬约束，没有快捷方式。

### 2.4 跨重启恢复

审批被挂起时（用户关掉应用 / 系统休眠），落库后下次启动会重武装：

- 窗口延长至 24 小时（默认 1 小时）
- 「上次那个命令还在等你决定」提示在聊天页顶部
- 决策路由到 `ResumeRun`，从检查点续跑

## 3. 目录信任

工作区外的目录首次写入前必须显式登记（fail-closed）：

- 设置页 → **安全 → 目录信任** 登记
- 「祖先信任」自动继承（信任 `/work/code/` 自动信 `/work/code/myapp/`）
- 撤销信任立即生效（下次 exec 重新问）

## 4. 用户钩子（自定义拦截点）

设置页 → **能力 → 钩子** 可以为 7 类生命周期事件注册用户命令子进程：

| 事件 | 时机 | 常见用途 |
|---|---|---|
| `SessionStart` | 会话首轮启动 | 注入项目特定上下文 |
| `UserPromptSubmit` | 用户消息入队前 | 拦截敏感输入、补充上下文 |
| `PreToolUse` | 工具执行前 | 自定义命令级拦截 / 改参数 |
| `PermissionRequest` | 审批前 | 自定义审计 / 自动放行 |
| `PostToolUse` | 工具执行后 | 注入附加上下文到回执 |
| `PostToolUseFailure` | 工具失败后 | 注入修复提示 |
| `Stop` | run 正常收尾时 | 自动续跑反馈 / 后处理 |

钩子通过 stdin 收 JSON 载荷，stdout 出 JSON 决策（`{decision: "allow|ask|deny", reason: "..."}`）。
钩子自身故障不阻断 run（内部已兜底放行）。

## 5. 反幻觉核验

`verifyArtifactClaims` 在 run 收尾时检查：

- 模型回复里声称「已创建 `report.pdf`」
- 但本 run 的 `file_changes` 表里没有对应记录
- → 自动发 `chat:warn{ kind: "unbacked_claim" }`

这是「让模型对过程负责」的硬约束——写一个文件必须真写了对应工具调用 + 真产生了文件变更。

## 6. 故障排查

| 症状 | 检查 |
|---|---|
| 审批窗口打不开 | 应用是否在前台运行；审批记录是否超时（24h）|
| `本会话允许` 不生效 | 命令是否带参数变化（`grants` key 含归一化参数）；是否被 `destructive` 拦截 |
| 钩子拦截没生效 | 查看 `%APPDATA%/WorkBaby/logs/` 的 hook 错误行；子进程是否可执行 |
| 反幻觉误报 | 检查 `file_changes` 行是否在该 run 内（跨 run 的文件变更不算证据）|