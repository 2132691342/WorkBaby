# 工作区与 Git · WORKSPACE

`wb-git` 提供工作区版本视图：读 status / diff / 历史 / 分支，写限白名单动作。
git 定位：内置 PortableGit 优先，其次系统 PATH；不做对象操作。

**工作区 = 对话的 cwd**：内核的工作目录、文件树、git 视图三者同源。
这不是巧合——助手改的文件、界面显示的文件、git 报告的文件必须是同一批。
所以界面上的工作区只有一页：左栏文件树（只读浏览 + 就地预览）、
中栏改动清单 / 提交 / 历史 / 分支，右栏差异。文件浏览不单开一页。

## 工作区语义

- 启动时从设置 KV `workspace` 解析：目录仍在且**适合作工作区**就用它；
  否则改用壳传入目录（桌面进程 cwd / standalone 的 argv），仍不合适就换成
  用户文档目录并写回 KV
- **切目录热生效**：改选（工作目录芯片或设置页行为面板）调 `POST /workspace/switch`，
  后端重扫技能与工作区区、重建待办存储与工具注册表、重连 MCP、写回 KV，
  并向全局通道广播 `app:workspace`；前端接到就重读引导与清单
- 有会话正在跑时拒绝切换（运行中的工具 cwd 与索引都对着旧目录），报 5001
- 系统目录与盘根不当工作区（`C:\Windows` / `System32` / `Program Files` / `C:\`）：
  从快捷方式启动时进程 cwd 往往就是这些位置
- 会话目录、fff 索引、知识库、git 视图都按工作区隔离，会话列表也只列当前工作区的
- 路径进出全部过 `wb_pkg::realpath`（短名与符号链接归一）再判越界；越界报 1003

## 读

| 接口 | 实现 |
|---|---|
| `GET /files` | 工作区目录列举（惰性：只读一层，不递归） |
| `GET /files/bytes` | 单文件字节（base64，文档预览用，上限 32MB） |
| `GET /git/status` | `status --porcelain=v1 -z` + 分支与领先落后（`rev-list --count`）；条目归类 staged / unstaged / untracked |
| `GET /git/diff` | 文件级 `diff --no-color`（未跟踪文件整篇算新增）；返回行结构（加 / 删 / 上下文 + 行号） |
| `GET /git/history` | `log`；单提交 `show --stat` 与文件 diff（`limit` 缺省 50） |
| `GET /git/file-history` | 单文件 `log --follow` |
| `GET /git/branches` | 本地与远端分支、当前分支、上游 |

**中文与空格路径**：用 `-z` 模式解析（NUL 分隔），展示层还原——
按行解析会在路径含换行时崩掉。

**未跟踪条目只归一条**（`?`），不产生空 status 重复行。
`MM` 是「已暂存 + 未暂存」两条，`UU` 标为冲突。

## 写白名单

界面能触发的 git 写操作是封闭集合，动作之外的参数一律拒绝：

| 动作 | 命令形态 | 护栏 |
|---|---|---|
| stage / unstage | `add --` / `restore --staged --` | 路径必须在工作区内 |
| discard | `restore --`（未跟踪文件走删除） | 危险确认气泡 |
| resolve | `add --`（冲突文件） | 仅中间态有意义 |
| commit | `commit -F <信息文件>` | 信息非空；`--amend` 仅限已有提交 |
| branch / switch | `branch` / `switch` | 分支名合法 |
| branch 删除 | `branch -d` | 只删已合并分支，拒绝 `-D` |
| fetch | `fetch [remote]` | 拉取远端更新，不动本地分支 |
| state | `merge --continue/--abort` · `rebase --continue/--abort` | 合并 / 变基中间态的继续与中止 |
| merge | `merge --no-edit` | 支持 `--rebase` 变体 |
| pull | `--ff-only`（缺省）/ `--rebase` | 分叉时报 1024 |
| push | `push` | 强制级别逐级升级，见下 |
| init | `init -b <branch>` | 目录为空或已是仓库 |
| remote | `remote add` | URL 合法 |

**危险参数直接拒绝**：`reset --hard` / `clean -fd` / 裸 `--force` / `branch -D`。
这些不是「不给界面按钮」就够——助手用 bash 也能跑，所以执行层也要拦。

**信息走文件不走参数**：`commit -F <信息文件>` 而非 `commit -m "<消息>"`——
消息里有引号或换行时，参数传递会被 shell 解析搞坏。

**强制推送分级**：`force` 字段从弱到强分三档——

| 档 | 命令 | 护栏 |
|---|---|---|
| （不填） | 普通 push | 远端领先时报 1024，要求先 pull |
| `lease` | `--force-with-lease` | 用远端当前提交做租约校验 |
| `force` | `--force` | 必须带与分支名一致的 confirm 才放行 |

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
| git 子进程而非纯 Rust 实现 | 行为与用户终端一致，免大依赖 | 需要一份 git（内置 PortableGit 兜底，零配置） |
| git 退出全挂 CREATE_NO_WINDOW | 开一个菜单就闪一次黑色控制台是硬伤；探测程序改纯路径查找，不起子进程 | 新增子进程出口必须自觉对齐 |
| 白名单而非逐次审批 | 动作集合封闭，拒绝面清晰 | 新增写动作要改表 |
| `--ff-only` 作为默认 | 防误合并；需要合并时显式走 merge | 个别分叉场景多一步 |
| 拒绝危险参数而非不提供按钮 | 助手也能跑 git，执行层必须拦 | 参数白名单要维护 |
| 信息走文件不走参数 | 避免 shell 解析搞坏含引号的消息 | 临时文件要清理 |
