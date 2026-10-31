# 内置功能 · FEATURES

12 项能力原生实现进 `wb-features`，每项独立开关。

扩展能力不设运行时插件市场：能力面已知且封闭，运行时加载第三方代码会引入
无法在编译期发现的问题（加载失败、版本不兼容、行为不可预测）。
技能与 MCP 承接剩余的扩展需求——它们是「数据」而不是「代码」。

## 统一形态

```rust
pub struct FeatureCtx {
    /// 数据目录
    pub data_dir: PathBuf,
    /// 当前工作区
    pub workspace: PathBuf,
    /// SSE 出口
    pub emit: Emit,
}
```

service 装配时注入 `FeatureCtx`，功能代码不依赖 service（依赖方向保证）。

每项功能 = 0..n 个 `Tool` 实现 + 可选 UI 桥事件。
**功能代码不感知界面**，只发事件；界面按事件词汇渲染。

## 开关即装配边界

关掉的功能，其工具不进 schema、命令不注册、事件不再发。
实现方式是 service 装配时按 `feature_toggle` 表过滤后重建注册表——不是运行时判断。

| 特征 | code | 工具 | 默认 |
|---|---|---|---|
| 待办清单 | todo | `todo` | 开 |
| 结构化提问 | ask_user | `ask_user_question` | 开 |
| 模糊搜索 | fff | `fffind` / `ffgrep` | 开 |
| 上下文地图 | context_view | 无（`/context/breakdown` 端点） | 开 |
| 持久记忆 | memory | `memory_save` / `memory_search` / `memory_remove` / `memory_consolidate` / `session_search` | 关 |
| 联网访问 | web | `fetch_content` / `web_search` / `pdf_read` / `github_clone` / `video_understand` | 关 |
| 装载配置 | loadout | 无（预设管理端点） | 关 |
| 省上下文 | context_mode | `ctx_execute` / `ctx_batch_execute` / `ctx_index` / `ctx_search` | 关 |
| 语言服务 | lsp | `lsp_diagnostics` / `lsp_fix` | 关 |
| 计划评审 | plan | `plan_submit` / `plan_mark_done` / `plan_inbox` / `plan_annotate` / `review_diff` | 关 |
| 目标追踪 | goal | `create_goal` / `get_goal` / `update_goal` | 关 |
| 内省面板 | inspect | 无（`/inspect` 快照） | 关 |

**默认关的判断标准**：涉网（web）与起外部进程（context_mode / lsp）的功能默认关，
因为它们的行为依赖用户机上的环境与网络状况——开着但不可用比关着更让人困惑。
纯本地能力（todo / ask_user / fff / context_view）默认开。

## 逐项说明

### todo · 待办清单

维护一份跨上下文压缩存活的待办清单。

工具 `todo` 支持增删改查与 `blocked_by` 依赖图：被依赖项没完成前，下游项不能开始。
依赖图的三个拒绝条件——悬空 id、自环、成环——都在写入时报错。

状态存工作区级文件（不是会话条目），因为工具契约拿不到会话句柄。

### ask_user · 结构化提问

让模型在「该猜」的时候改为向你提问，而不是硬猜一个答案。

工具 `ask_user_question` 一次最多 4 题，每题 2-4 个选项 + 自由输入，支持多选，
选项可带 markdown 预览。工具调用会阻塞等待用户回答——等待经 `AnswerBoard` 桥接：
内核发起提问 → 发 `chat:ask` 事件 → 界面出卡片 → 用户提交 → `POST /chat/answer` 回填。

`AnswerBoard` 保证一次投递：重复回答、无人等待者的回答都返回失败。

### fff · 模糊搜索

模糊文件名（`fffind`）与模糊内容（`ffgrep`）搜索，容忍拼写误差与大小写。

用 nucleo 做模糊匹配，结果按 frecency（使用频率 + 最近访问时间）加权排序——
最近常用且常用的文件排前面。frecency 数据存在独立 SQLite 库里。

遍历工作区时跳过 `node_modules` / `target` / `dist` 等产物目录。

### memory · 持久记忆

`memory_save` / `memory_search` / `memory_remove` 三个工具管四条记忆流：
`memory`（一般事实）/ `user`（用户偏好）/ `project`（项目事实）/ `failure`（失败教训）。
存 SQLite FTS5 库，跨会话存活。

**写前密钥扫描**：写入前扫一遍内容，命中真实凭据格式（各类 API key、私钥块）
就拒绝写入。这条是硬门槛——记忆库是明文落盘的，密钥进去等于泄露。

`session_search` 检索历史会话正文——跨会话找回「上次那个结论」。

**自动知识整合**：记忆库只增不减，到一定量就会堆满互相重复的碎片——检索召回一堆近似内容，
上下文被占满，模型也分不清哪条更新。`memory_consolidate` 工具（以及写入时自动触发）
会按类别逐对判重，把说的是同一件事的条目合并成一条。

| 参数 | 值 | 理由 |
|---|---|---|
| 相似度阈值 | 0.5 | 字符级词元交并比 |
| 分词口径 | 中文按字，西文按词 | 中文不分词时改写句能对上，分词切错就完全对不上 |
| 特征词门槛 | 两段必须共用特征词 | 「我喜欢猫」与「我不喜欢狗」字面重合高但不是一回事 |
| 单次扫描上限 | 300 条 | 控制整合的扫描成本 |
| 自动触发阈值 | 40 条 | 到量就整合，不等模型手动叫 |
| 合并方向 | 保留较早创建的 | 早的通常是原始陈述，后的是补充 |

合并后条目更少、内容更全，检索质量才稳得住。

### web · 联网访问

五个工具：

| 工具 | 作用 |
|---|---|
| `web_search` | 联网搜索，返回编号的结果清单 |
| `fetch_content` | 抓取 URL，按类型分流（JSON 直通 / HTML 抽正文） |
| `pdf_read` | 抽取 PDF 文字（本地路径或远程地址） |
| `github_clone` | 浅克隆公开仓库到工作区 |
| `video_understand` | 取视频页面元信息与字幕轨文本 |

**搜索源**：默认依次尝试 `duckduckgo` → `bing` → `wikipedia`，第一个解析出结果即返回；
可用 `provider` 参数锁定单一源。每个源有自己的页面结构，适配器负责转成统一结果列表——
解析失败就换一个源，不编造结果。

**PDF 抽取**：走本地解析，文件不出机器。扫描版 PDF 没有文字层，如实说明并提示需要 OCR，
不返回空白冒充内容。远程 PDF 先下载再解析，大小上限 64MB。

**GitHub 克隆**：仓库标识必须是 `owner/name` 形态（拼 URL 前就卡住，不给注入留口子）；
目标目录只能落在工作区下且不允许已存在——避免覆盖用户已有内容。

**视频理解**：字幕是理解视频的主要入口。有字幕轨就抽字幕全文（按时间轴分段），
没有就返回页面元信息并明确说明。语言偏好按标签与地址里的语言标记两处匹配。

### context_mode · 省上下文

核心思路：**别让长内容占着上下文**。

| 工具 | 作用 |
|---|---|
| `ctx_execute` | 在沙箱里跑代码，只有 stdout 进上下文（stderr 折叠为附注） |
| `ctx_batch_execute` | 一次跑多条命令，只回汇总 |
| `ctx_index` | 把长内容收进知识库，只返回文档句柄不回吐正文 |
| `ctx_search` | 在知识库里检索片段，返回摘要与来源 |

沙箱支持 12 种语言（javascript / typescript / python / shell / powershell /
ruby / php / perl / go / dotnet / java / lua），脚本落盘后用对应解释器执行，
执行完即删。解释器不在 PATH 上时报 7200（沙箱解释器不可用）。

知识库是 SQLite FTS5，正文按段落切块入库。检索用**双路召回 + RRF 融合**：
一路 FTS5 词面匹配（BM25 排序），一路逐字 LIKE（覆盖缩写与专有名词），
两路结果按倒数排名融合（RRF）——单路召回会漏掉另一种匹配方式能命中的内容。

### lsp · 语言服务

`lsp_diagnostics`（工作区静态诊断）与 `lsp_fix`（快速修复）。

自带 stdio JSON-RPC 客户端（按 Content-Length 分帧），配 15 个语言服务器：
rust-analyzer / gopls / ruff / ty / biome / typescript-language-server / clangd /
jdtls / yaml-language-server / lua-language-server / intelephense / zls /
bash-language-server / terraform-ls / rubocop。

每个服务器带自己的扩展名映射与跳过目录（如 rust-analyzer 跳过 `target` 与 `.git`）。

**server 按需启停**：一次工具调用内启动、用完即关，不留常驻进程。
语言服务器需用户自装（不下载），未装的语言在诊断结果里列为「跳过」。
一台 server 都没装时功能开关即使打开，工具也不进注册表（避免死工具）。

`lsp_fix` 默认只预览不写盘——模型拿到预览后自己判断，再传 `write=true` 落盘。

### plan · 计划评审

把多步执行计划提交给用户评审（批准 / 驳回），适合改动面大、有破坏性或方向性选择的任务。

`plan_submit` 阻塞等待裁决；驳回会带意见返回，修订后可重新提交。
执行期用 `plan_mark_done` 勾选步骤，状态以 widget 形式推给界面。

**标注**：`plan_annotate` 给计划或某一步加一条意见（`scope=plan` / `scope=step`）。
标注只记录意见，不改变步骤状态——要终止计划走驳回。标注与计划一起呈现，
下一轮助手会看到用户对哪一步有疑问。

**PR 审查**：`review_diff` 拿一个分支相对基线的 diff 出来审查。
PR 的内容在本地就是「基线分支 vs 特性分支」的差异，所以不需要连 GitHub API，
也不需要用户配令牌。基线缺省取与当前分支的共同祖先（merge-base），否则用 `origin/main`。
返回变更统计与逐文件 diff——改动面大时先看统计再挑重点文件。

### goal · 目标追踪

登记一个需要跨多轮推进的目标（同一时间只有一个活跃目标）。
多轮长任务、被中断后要续跑的任务用它记录目标与步骤。

状态机：`active` / `blocked` / `achieved` / `abandoned`。
从 `blocked` 恢复须显式置回 `active`——不自动恢复，避免助手在用户没介入时继续往下走。

**自动续跑**：一轮对话结束后，如果目标仍 `active` 且还有未完成步骤，助手会自己开下一轮
接着推进（走内部运行路径，不占用界面输入框）。续跑提示里带目标描述、已完成计数与待办步骤清单。

| 约束 | 值 | 理由 |
|---|---|---|
| 连续续跑上限 | 20 轮 | 模型没推进目标却一直挂着目标时不能无限转下去 |
| 触发条件 | 活跃 + 有未完成步骤 | 没步骤清单就无从判断「还差什么」 |
| 计数重置 | 用户自己发消息时归零 | 人介入了，计数从头算 |

达到上限时停手并提示用户目标仍未完结——不静默放弃。

状态行随变更推给界面（footer 显示目标与进度）。

### loadout · 装载配置

工具与技能开关的命名预设。`full` / `minimal` 两个内置，其余用户自存。

本质是把「开哪些工具与技能」这套组合命名下来，方便在「全能」与「精简」之间快速切换。

### inspect · 内省面板

一屏看全当前装配：工具清单、技能清单、斜杠命令、系统提示词全文、功能开关、会话元信息。
快照数据走 `/inspect` 端点。

### context_view · 上下文地图

把上下文占用拆成分段：系统提示词 / 工具定义 / 历史消息各占多少 token。
数据走 `/context/breakdown` 端点。

## UI 桥

功能不直接操作界面，只发事件。界面按统一词汇渲染：

| 内核侧动作 | SSE 事件 | 界面呈现 |
|---|---|---|
| 提问 | chat:ask / chat:ask_closed | 提问卡 |
| 状态行（空文本即清除） | chat:status | 状态条 |
| 小部件行（空行组即清除） | chat:widget | 状态条 |
| 通知（info/warning/error） | chat:notify | Toast |
| 标题建议 | chat:title | 会话名 |

## 存储位置

| 功能 | 位置 |
|---|---|
| todo / plan / goal | `{数据目录}\features\{编码后工作目录}\*.json` |
| memory | `{同上}\memory.db`（FTS5） |
| fff | `{同上}\frecency.db` + 查询历史 |
| context_mode | `{同上}\knowledge.db`（FTS5）+ `sandbox\` |
| web | `{同上}\web\`（结果缓存） |

功能各自开自己的库文件，不进 `wb-repo` 的主库——主库管设置与索引，
功能数据有自己的生命周期与清理策略。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 原生化而非运行时加载代码 | 加载失败、版本不兼容、行为不可预测都在编译期消失 | 失去第三方生态 |
| 开关即装配边界 | 关掉的功能不进 schema，模型根本看不到 | 切换要重建注册表 |
| 功能不依赖 service | 依赖方向单向，功能可单独测 | 功能拿不到 service 的能力，只能靠 FeatureCtx |
| 涉网与外部进程默认关 | 环境不可用时开着的功能更让人困惑 | 用户要多点一次 |
| UI 走事件桥而非直接调界面 | 内核与功能都不含界面词汇 | 事件词汇要维护 |
