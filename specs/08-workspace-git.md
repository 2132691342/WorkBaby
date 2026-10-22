# 08 · 工作区版本视图

## 定位

助手会在用户的文件夹里读写文件、跑脚本、改代码。**「它到底改了什么」必须能被看见**——
看不见就只剩下信任，而信任在第一次改坏文件之后就归零了。

这一块回答五个问题：改了哪些文件、每个文件改了什么、这个仓库发生过什么、
把眼前的改动收成一个提交，以及卡在合并 / 变基中间时怎么走出去。
看的永远是助手实际干活的那个目录。

**读全开放，写受控**：状态、差异、历史、文件历史、分支都是只读；
写入每一种都**由用户点下去**，助手不代提；不可逆的动作（丢弃、强制推送）必须显式确认。

## 设计

| 决策 | 选择 | 为什么 | 代价 |
|---|---|---|---|
| 读写范围 | 读全开放；写限白名单动作 | 「准备一次提交」是办公闭环的一部分，但写版本历史不可逆 | 要维护白名单与一组确认态 |
| 命令白名单 | 显式列举允许的子命令与参数 | 字符串拼命令等于把 `--force` 一起放进来 | 加一种动作要改一处白名单 |
| 实现方式 | 调系统 `git` 命令 | 仓库状态与差异的语义由 Git 定义，自己解析 `.git` 等于重实现对象格式与索引 | 机器上得装 Git；每次读有进程开销（毫秒级，有超时兜底） |
| 提交信息 | 用户填；可让助手按 diff 起草 | 提交信息是这次改动的摘要，作者只能是用户 | 多一次模型调用与一条失败退路 |
| 依赖方向 | `backend/git` 是能力域；`service.GitService` 装配，`api` 只做参数与响应 | 与技能同一条链路，不新增一条旁路 | 多一层 service（但工作目录校验只该有一处） |
| 失败表达 | 没装 Git / 不是仓库 = **状态**不是错误 | 处置动作是「去装 / 去初始化」，报错只会让人反复点刷新 | 前端要判 `available` / `is_repo` 而不是只看 code |
| 路径约定 | 界面一律用**工作区相对**路径；git 命令一律在**仓库根**执行、参数是根相对 | 两者之差（仓库前缀）只在 `backend/git` 内换算：界面把 status 给的路径原样回传就能用，不必自己拼前缀 | 每个子命令前多一次 `rev-parse`（有缓存价值：一次操作只探一次） |
| 中间状态 | merge / rebase 冲突**不是失败**，是可继续的状态 | 「解决冲突再继续」本来就是一串界面动作；报错会把用户踢回命令行 | 状态里有 `state` 与逐文件 `conflict`，前端要多渲染一条横幅 |
| 强制推送 | 分三级：`none` → `lease` → `force` | `--force` 会静默覆盖别人的提交；分级让「安全的那一档」是默认路径 | 界面上多一块默认收起的危险区 |
| 输出归因 | stdout 与 stderr **都**收，失败时合并上报 | git 把「无改动可提交」「没有上游分支」写在 stdout，只看 stderr 就只剩 `exit status 1` | 错误文案可能同时含两段输出 |

`backend/git` 不依赖任何业务包（只引 `domain` 与 `pkg`），因此可以被单测直接驱动——
路径换算、差异解析与写入护栏是这个包里最容易写错的三处，都必须有测试。

### 写入白名单

允许的动作及其参数：

| 动作 | 实际命令 | 约束 |
|---|---|---|
| 入栈 | `add -- <path>` | 路径必须落进工作目录；`--` 之后的路径不做通配展开 |
| 出栈 | `reset HEAD -- <path>` | 同上；只动索引，不动工作区文件 |
| 标记冲突已解决 | `add -- <path>` | 同上；解决后冲突标记消失，但还没提交 |
| 丢弃改动 | `checkout -- <path>` + 删未跟踪文件 | **必须显式给路径**，不提供「全部丢弃」 |
| 提交 | `commit -m <msg>` | 信息非空、≤ 5000 字符；不加 `--no-verify` |
| 改写上次提交 | `commit -m <msg> --amend` | 同上；界面必须二次确认并提示「已推送的不要改写」 |
| 新建分支 | `checkout -b <name>` | 名字只允许 `[A-Za-z0-9._/-]`、≤ 128 字符、不以 `-` 开头 |
| 切分支 | `checkout <name>` | 名字同上；不带 `--force`、不带 `-f` |
| 删除分支 | `branch -d <name>` | 只删**已合并**的分支；`-D` 不开放 |
| 合并 | `merge --no-edit <name>` | 冲突留下未完成合并（返回状态，不报错） |
| 变基 | `rebase <name>` | 同上 |
| 继续 / 中止 | `commit --no-edit` / `merge --abort` / `rebase --continue` / `<kind> --abort` | 只在有中间状态时可用 |
| 拉取 | `pull --no-edit [<remote> [<branch>]]` | 冲突同样返回状态；全程禁交互式提问 |
| 取回 | `fetch --prune [<remote>]` | 不动工作区与本地分支；推送被拒后先看远端多了什么 |
| 推送 | `push [--set-upstream] [--force-with-lease] [--force] <remote> <branch>` | 见下面「强制推送分级」 |
| 初始化 | `init [--initial-branch=<name>]` | 已有仓库时不覆盖用户的分支配置 |
| 添加远端 | `remote add <name> <url>` | 名字 `[A-Za-z0-9._-]`；地址只放 https / ssh / git / file / scp / 本地路径 |

明确禁止（白名单之外一律拒绝 1018，不解析、不猜测）：

| 禁止 | 原因 |
|---|---|
| `reset --hard` | 静默毁掉工作区里未提交的改动 |
| `branch -D` / `-M`（强删、强改指向） | 删未合入分支、改写分支指向 |
| `clean -fd` | 静默删除未跟踪文件 |
| `commit --no-verify` | 跳过钩子不该由界面替用户决定 |
| `checkout --force` / `-f` | 用索引覆盖工作区改动 |
| `cherry-pick` / `revert` / `stash` / `reset --soft` | 目前不开放；它们的中间状态与冲突处置各有一套语义，界面没有对应引导 |
| 任何 `-C <dir>` / `--git-dir` / `--work-tree` | 换目录等于绕过工作区校验 |
| 以 `-` 开头的分支名 / 远端名 / 地址 | 会被 git 当成选项（`--upload-pack=<cmd>` 这类是命令执行面） |

### 强制推送分级

推送被拒（`1024`）之后，界面上是一条从安全到危险的选择链：

| 级别 | 命令 | 语义 | 界面要求 |
|---|---|---|---|
| `none` | `push` | 远端有本地没有的提交就拒绝 | 默认路径 |
| `lease` | `push --force-with-lease` | 本地远端跟踪引用与远端不一致就拒绝 | 危险区里可选中 |
| `force` | `push --force` | 无条件覆盖远端历史 | 必须输入分支名确认 |

**故意不先 fetch 再带租约推送**：租约比的就是本地跟踪引用，那个引用只在上一次同步时更新过。
先 fetch 等于把租约变成无条件覆盖——而「我上次同步之后没人推过」正是租约的全部价值。
要在「已知远端现状」的前提下强制推送，用户先点「取回远端」，再带租约推送。

## 契约

| 方法 | 路径 | 出参 |
|---|---|---|
| GET | `/api/v1/git/status?session_id=` | `GitStatusVO` |
| GET | `/api/v1/git/diff?path=&session_id=` | `GitDiffVO` |
| GET | `/api/v1/git/history?limit=&session_id=` | `GitCommitVO[]` |
| GET | `/api/v1/git/commit?hash=&session_id=` | `GitCommitDetailVO` |
| GET | `/api/v1/git/file-history?path=&limit=&session_id=` | `GitCommitVO[]` |
| GET | `/api/v1/git/branches?session_id=` | `GitBranchVO[]`（本地 + 远端） |
| POST | `/api/v1/git/stage` | `{session_id?, paths[]}` → `GitStatusVO` |
| POST | `/api/v1/git/unstage` | `{session_id?, paths[]}` → `GitStatusVO` |
| POST | `/api/v1/git/discard` | `{session_id?, paths[]}` → `GitStatusVO`（`paths` 不能为空） |
| POST | `/api/v1/git/resolve` | `{session_id?, paths[]}` → `GitStatusVO` |
| POST | `/api/v1/git/commit` | `{session_id?, message, amend?}` → `GitCommitRESP` |
| POST | `/api/v1/git/branch` | `{session_id?, name}` → `GitStatusVO`（新建并切过去） |
| POST | `/api/v1/git/branch/delete` | `{session_id?, name}` → `GitStatusVO` |
| POST | `/api/v1/git/switch` | `{session_id?, name}` → `GitStatusVO` |
| POST | `/api/v1/git/merge` | `{session_id?, name, rebase?}` → `GitStatusVO` |
| POST | `/api/v1/git/state` | `{session_id?, action: continue\|abort}` → `GitStatusVO` |
| POST | `/api/v1/git/pull` | `{session_id?, remote?, branch?, rebase?}` → `GitStatusVO` |
| POST | `/api/v1/git/fetch` | `{session_id?, remote?}` → `GitStatusVO` |
| POST | `/api/v1/git/push` | `{session_id?, remote?, branch?, set_upstream?, force?, confirm?}` → `GitStatusVO` |
| POST | `/api/v1/git/init` | `{session_id?, branch?}` → `GitStatusVO` |
| POST | `/api/v1/git/remote` | `{session_id?, name, url}` → `GitStatusVO` |
| POST | `/api/v1/git/message` | `{session_id?, paths[]?}` → `{message, err}` |

`session_id` 决定看哪个目录：给了就看**该会话的工作目录**，没给（或会话已被删）退回全局工作目录。
只用全局会看错地方——会话级目录是这个会话里改动真正发生的地方。

所有写端点都返回**写后重读的 `GitStatusVO`**：让界面用一份数据刷新列表，
而不是前端自己推断「刚才入栈的那个文件现在应该长什么样」。

`GitStatusVO` 的要点：

| 字段 | 含义 |
|---|---|
| `available` / `reason` | 有没有 git、不可用时的一句话原因 |
| `is_repo` | 工作目录是否在仓库里 |
| `workspace` / `repo_root` | 看的目录 / 仓库根（工作区是子目录时两者不同） |
| `branch` / `upstream` | 当前分支 / 上游引用（空表示没有） |
| `ahead` / `behind` | 相对上游的领先 / 落后数 |
| `state` / `state_detail` | `none` / `merge` / `rebase` / `cherry-pick` / `revert` + 一句话说明 |
| `remotes[]` | `name` / `url` |
| `conflict` | 当前是否有未解决的冲突 |
| `files[]` | `path` / `status` / `staged` / `conflict` / `additions` / `deletions` |

`files[].status` 取 `M` 改过 / `A` 新增 / `D` 删除 / `R` 改名 / `C` 复制 / `T` 类型变化 /
`?` 未跟踪 / `U` 冲突；`staged` 区分「已放进待提交区」与「只改了工作区」。
`lines[]` 的 `kind` 取 `file` 文件头 / `hunk` 段头 / `add` / `del` / `ctx`，
`old_no` / `new_no` 为 0 表示该侧没有对应行。

错误码段位与处置动作见 [ERROR-CODES](../docs/ERROR-CODES.md)：本域是 `1015`–`1027`，
路径越界走 `1004`、工作目录不存在 `1003`、文件不存在 `1005`。

## 关键流程

### 看改动

1. 前端进 `/workspace` → `GET /git/status`
2. 不可用（`available=false`）→ 空态卡说「去装 Git」
3. 不是仓库（`is_repo=false`）→ 空态卡给「初始化仓库」输入框与按钮，一次点完
4. 可用 → 中间状态横幅（若有）→ 分支条 + 改动列表，同时拉「整个工作区」的差异
5. 点某一行 → `GET /git/diff?path=…`，行上转圈、右栏换成这份差异
6. 读失败 → toast 说清是哪个文件读不出来（列表那一行还亮着，不说就是「点了没反应」）

状态**不自动轮询**：仓库状态不会自己变，变的只有助手的行为；刷新是显式按钮。

### 提交

1. 用户在改动列表里勾选要入这次提交的文件 → `POST /git/stage`
2. 想省事：点「让助手起草」→ `POST /git/message`，信息框填入草稿（**可改，不自动提交**）
3. 需要改写上一次提交时勾「改写上次提交」，旁边立刻出现代价说明
4. 确认提交 → `POST /git/commit`
5. 成功 → 列表刷新（空改动）+ toast 带上提交号
6. 失败 → 按错误码给出可执行的下一步（无改动 / 缺身份 / 有冲突 / 钩子拒绝）

**提交前一定列出文件与增删行数**：用户看到的「提交」两个字背后是哪些文件，
必须在这一刻可见；看不见就会把不想提交的文件一起带进去。
**有冲突时提交按钮禁用**，并说明「先标记已解决」——点了必然失败，不如不给点。

### 处理冲突

1. 合并 / 变基 / 拉取返回的 `state != none` 且 `conflict = true`
2. 顶部横幅显示「正在合并 xxx」+「N 个文件还有冲突」+「继续 / 中止」
3. 改动列表里冲突文件带 `U` 标记与「标记已解决」按钮
4. 用户在编辑器里改好内容 → 点「标记已解决」→ `POST /git/resolve`
5. 全部标记完 → 横幅上「继续」可用 → `POST /git/state`（`continue`）
6. 想放弃 → 横幅上「中止」→ 二次确认 → `POST /git/state`（`abort`）

**「继续」在还有冲突时禁用**：git 会在这一步失败，让按钮先灰掉比让用户吃一次报错好。

### 推送与强制推送

1. 「推送」→ `POST /git/push`（空 body）；没有上游时后端自动 `--set-upstream`
2. 被拒（`1024`）→ toast 说明「远端有本地没有的提交」
3. 用户可以先「取回远端」看清别人推了什么，或者拉取合并
4. 确实要覆盖 → 展开危险区，选级别
   - 带租约：直接执行（远端变了就会被拒）
   - 无条件：必须先输入当前分支名，按钮为危险色

### 看历史

1. 历史页 → `GET /git/history`，一页若干条
2. 点一条 → `GET /git/commit?hash=…`，抽屉里显示这次提交改了哪些文件与逐行差异
3. 在某个文件的差异上点「这个文件的历史」→ `GET /git/file-history?path=…`

## 不变量

| 不变量 | 锁死原因 |
|---|---|
| 界面路径是工作区相对；git 参数是仓库根相对，换算只在 `backend/git` 内 | 直接回传工作区相对路径会变成 `sub/sub/a.txt`，子目录工作区里暂存与差异必然失败 |
| 所有命令在仓库根执行 | 在子目录执行时 `status` 报的是仓库根相对路径，与界面约定不同源 |
| 路径必须落进工作目录 | 工作区之外的文件不该被这个入口读到，写入时更要命 |
| 折相对路径前两侧都先过 `pkg.RealPath` | 同一份路径的短名（`RUNNER~1`）与长名、符号链接形式字面不等，直接 `filepath.Rel` 会算出 `../..` |
| 写命令走白名单，参数逐个校验 | 字符串拼命令会把 `--force` 一起放进来，而它的后果不可逆 |
| 失败归因合并 stdout 与 stderr | git 把「无改动」「无上游」写在 stdout，只看 stderr 只会报 `exit status 1` |
| 退出码为 0 而命令没跑起来（超时）不当 1019 | 那说明不了 git 拒绝了什么，当成业务失败会误导处置 |
| 提交信息非空且 ≤ 5000 字符 | 空信息让这次提交在历史里无法辨认 |
| 冲突是状态不是错误 | 「解决冲突再继续」是串界面动作，报错会把用户踢回命令行 |
| 有冲突时提交按钮禁用、「继续」禁用 | 这两个动作在冲突未解决时必然失败 |
| 丢弃改动必须显式给路径 | 空 paths 的丢弃等于一次不可逆清空 |
| 强制推送默认不存在；纯 `--force` 要输分支名确认 | 它会静默覆盖别人的提交 |
| 带租约的强制推送**不**前置 fetch | 租约比的是本地跟踪引用，先 fetch 会把「没人推过」这个前提抹掉 |
| 删除分支只走 `-d` | `-D` 会删掉未合入的分支，那些提交随即无引用 |
| 推送 / 拉取全程 `GIT_TERMINAL_PROMPT=0` | GUI 里没有终端，等凭据输入的结局是界面卡到超时 |
| 变基用 `GIT_EDITOR=:` | 不设就会拉起编辑器，界面永远等下去 |
| 差异有 3000 行 / 256KB 双上限，截断必须回报 `dropped` | 静默截断会让用户以为文件就这么长 |
| 未跟踪文件整篇按新增行读出（单篇与整篇差异都要） | `git diff` 对新文件给不出内容，「新建的文件」否则永远看不到 |
| `@@` 头给出的就是首行行号（先落号再推进） | 差一位就是整块行号整体偏移 |
| 命令带超时（status 10s / diff 15s / 写 60s / 推送拉取 120s） | 大仓库不该把界面卡住；推送可能真的慢，给足 |
| 没装 Git 与「不是仓库」不返回错误 | 它们的处置动作是「去装 / 去初始化」，不是重试 |
| 写后一律重读状态返回 | 前端自己推断状态会出现「界面说已入栈、git 说没有」 |
| 助手只起草提交信息，不提交 | 提交是用户的决定；助手代提会让「这次改动是谁的意思」说不清 |

## 取舍

- **写限白名单动作**：换来「准备一次提交 + 合并 + 推送」在应用内闭环；
  代价是 `cherry-pick` / `revert` / `stash` / `reset --soft` 仍然要回用户自己的 Git 工具，
  且每加一种动作都要过一遍「它能不能不可逆地毁东西」
- **开放强制推送但分级**：`--force-with-lease` 是安全的（远端变了就失败），
  无条件 `--force` 用「输分支名」换一次主动确认；代价是危险区要多维护一套确认态
- **冲突留在应用内解决**：比塞一句「请用命令行」好，但要多维护 `state` 一整套字段与横幅，
  且必须承认一件事——真正的冲突解决要在编辑器里改文件，界面只能做「标记 + 继续」
- **子目录工作区统一换算到仓库根**：换来界面路径与 git 路径的单一约定；
  代价是每次操作多一次 `rev-parse`，且 `repo_root` 与 `workspace` 会在界面上并存（可能让用户困惑）
- **提交信息可助手起草**：省掉想一句话的工夫，diff 就在手里；
  代价是多一次模型调用与一条失败退路（起草失败就手写，不许空着提交）
- **历史与文件历史是实时 `git log`**：不另建一张提交表；
  代价是深历史翻页有进程开销，且换分支后历史整块变（这正是不该缓存的原因）
- **走系统 Git**：零解析成本、语义永远与 Git 一致；
  代价是首屏要处理「没装 Git」这条分支
- **差异在后端折成行结构**：前端不必懂统一差异格式，两处配色规则也只在一处实现；
  代价是出参比原始文本大，且截断策略由后端决定（界面只能照实说）
