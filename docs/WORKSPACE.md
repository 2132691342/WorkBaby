# 工作区与 Git · WORKSPACE

`wb-git` 提供工作区版本视图：读 status / diff / 历史，写限白名单动作。
走系统 git（PATH 探测），不做对象操作。

**工作区 = 对话的 cwd**：内核进程的工作目录、文件树、git 视图三者同源。
这不是巧合——助手改的文件、界面显示的文件、git 报告的文件必须是同一批。

## 工作区语义

- 全局一个当前工作区（settings KV `workspace` + 历史表）
- 切换时机：内核空闲才允许（会话忙时拒绝）；切换后重扫会话目录、重建文件树与 git 视图
- 路径进出全部过 `wb-pkg::realpath`（短名与符号链接归一）再判越界；越界报 1003

## 读

| 接口 | 实现 |
|---|---|
| `/git/status` | `status --porcelain=v1 -z` + 分支与领先落后（`rev-list --count`）；条目归类 staged / unstaged / untracked |
| `/git/diff` | 文件级 `diff --no-color`（未跟踪文件整篇算新增）；返回行结构（加 / 删 / 上下文 + 行号） |
| `/git/history` | `log` 分页；单提交 `show --stat` 与文件 diff |
| `/git/file-history` | 单文件 `log --follow` |
| `/git/branches` | 本地与远端分支、当前分支、上游 |

**中文与空格路径**：用 `-z` 模式解析（NUL 分隔），展示层还原——
按行解析会在路径含换行时崩掉。

**未跟踪条目只归一条**（`?`），不产生空 status 重复行。
`MM` 拆成两条（已暂存 + 未暂存），`UU` 标为冲突。

## 写白名单

界面能触发的 git 写操作是封闭集合，动作之外的参数一律拒绝：

| 动作 | 命令形态 | 护栏 |
|---|---|---|
| stage / unstage | `add` / `restore --staged` | 路径必须在工作区内 |
| discard | `restore`（工作区） | 未跟踪文件删除；危险确认气泡 |
| resolve | `add`（冲突文件） | 仅合并 / 变基中间态 |
| commit | `commit -F <信息文件>` | 信息非空；`--amend` 仅限 HEAD 提交 |
| branch | `branch` / `switch` | 分支名合法 |
| merge | `merge` | 默认 `--ff-only` |
| pull | `pull --ff-only` | 同上 |
| push | `push` | 强制推送需显式分级与租约确认 |
| init | `init` | 目录为空或已是仓库 |
| remote | `remote add/remove` | URL 合法 |

**危险参数直接拒绝**：`reset --hard` / `clean -fd` / `--no-verify` / `branch -D`。
这些不是「不给界面按钮」就够——助手用 bash 也能跑，所以执行层也要拦。

**信息走文件不走参数**：`commit -F <信息文件>` 而非 `commit -m "<消息>"`——
消息里有引号或换行时，参数传递会被 shell 解析搞坏。

## 冲突中间态

`merge` / `rebase` 失败后读 `MERGE_HEAD` / `rebase-merge` 判定中间态，
界面显示冲突横幅。

中间态里：提交与「继续」都要求冲突文件全部 resolve（否则报 1021）；
「中止」恢复原状。逐文件解决 = 对冲突文件 resolve（stage），
全部解决后可继续或提交。

## 助手协作

提交信息可以让助手起草——这是 chat 通道外的短调用（内核以无会话短轮询生成，
不落会话文件），`POST /git/message` 即此通道。

助手用 bash 跑 git 是普通工具行为，不受白名单管——白名单管的是**界面写操作**。
两者边界清晰：界面上的每个按钮都有明确的确认语义，而助手自己执行的命令
由模型的判断与用户的授权范围共同约束。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 系统 git 而非纯 Rust 实现 | 行为与用户终端一致，免大依赖 | 依赖用户装 git |
| 白名单而非逐次审批 | 动作集合封闭，拒绝面清晰 | 新增写动作要改表 |
| `--ff-only` 作为默认 | 防误合并；需要合并时显式走 merge | 个别分叉场景多一步 |
| 拒绝危险参数而非不提供按钮 | 助手也能跑 git，执行层必须拦 | 参数白名单要维护 |
| 信息走文件不走参数 | 避免 shell 解析搞坏含引号的消息 | 临时文件要清理 |
