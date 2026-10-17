# 03 · Skill 系统

## 定位

Skill = 一段按需注入的领域知识（prompt 素材），不是可执行插件。
新手小白的「秘籍」：让助手在特定场景按固定套路干活。
纯文本，**安全面为零**——不产生任何执行行为，新手也能自己写。

**文件的真身在 PI 的技能目录里**。WorkBaby 是它的编辑器与文件管理器，
不是第二个加载器：PI 自己扫目录、自己渲染清单、自己决定什么时候读正文。

## 文件格式

```
assets/skills/{name}/SKILL.md                 # 内置（embed），启动时落到磁盘
{数据目录}/skills/{name}/SKILL.md               # 用户全局（= PI 的 <agent-dir>/skills/）
{workspace}/.pi/skills/{name}/SKILL.md         # 工作区级（PI 自己会扫）
```

`<agent-dir>` 由 `PI_CODING_AGENT_DIR` 指向 `{数据目录}/pi-agent`（见 [26](26-pi-bootstrap.md)），
所以 `{数据目录}/skills/` 与 PI 的用户技能目录是同一个地方。

内置技能虽然在源码里是 embed 资源，启动时必须先**写一份到磁盘**再登记：
PI 与模型都是按路径去读正文的，虚拟路径在文件系统里不存在。
写失败时退化为「正文只留在内存」，界面里点开仍能看到内容，但 PI 那边看不到这个技能。

```markdown
---
name: weekly-report
when_to_use:
  - 周报
  - 写周报
description: 按公司模板把本周工作整理成周报。
---
（正文：给模型看的操作指引，Markdown）
```

`name`（小写字母 / 数字 / 连字符，≤64）不合规才拒绝；`name` 与 `description`
缺失时有兜底（分别回退目录名与正文首行）。`when_to_use` 是可选触发词列表。

这套 frontmatter 格式与 PI 的 skill 规范一致，因此**同一个文件两边都能读**。

## 三个来源与优先级

| source | 目录 | 同名冲突 |
|---|---|---|
| builtin | 数据目录下的内置落盘区 | 低 |
| global | 用户技能目录 | 中 |
| workspace | `<workspace>/.pi/skills/` | 高 |

`List()` 全部列出并带 `source`，让用户看得见「这个技能是从哪来的」。
工作区级读 PI 的技能目录 `<workspace>/.pi/skills/`：技能加载本就是 PI 的能力，
同一份目录两边都读，用户用 PI CLI 放进去的技能，界面里立刻看得见。

## 注入方式

三条路径，都指向同一个 SKILL.md：

| 入口 | 机制 |
|---|---|
| PI 自己判断该用 | PI 的清单注入 + 模型自己读正文（渐进式披露） |
| 界面点技能卡片 | WorkBaby 把正文拼进当前这条用户消息 |
| 斜杠命令 | 同「点卡片」，WorkBaby 侧展开 |

**界面点卡片走的是消息注入而不是斜杠命令**：PI 的 `/skill:name` 是 TUI 的输入语法，
RPC 的 `prompt` 命令里它会被当作普通文本。要立刻按某个技能干活，
把正文拼进 `message` 是这个协议下唯一可靠的方式。

正文注入后仍在 `entries`（PI 的 JSONL）里是一条普通 user 条目——
不是新的消息类型，回放与导出都不用特判。

## 管理

| API | 说明 |
|---|---|
| GET /skills | 列表（含来源与启停状态） |
| POST /skills | 新建：写 `SKILL.md` 到全局技能目录 |
| POST /skills/import | 从磁盘导入 `{paths[]}`：单个 SKILL.md、含它的文件夹，或装满技能子目录的目录（只扫一层）；同名自动追加 `-2` 序号不覆盖；返回 `{imported, skipped[]}`（skipped 是没导进来的条目与原因，界面要能看到，静默丢弃会让用户以为文件放错了） |
| POST /skills/import-zip | 导入压缩包 `{filename, data}`（zip 字节 base64）：解压到临时目录后走同一条定位复制链路；条目路径经 SafeJoin 校验防穿越，单文件 64MB / 总量 256MB 双限流 |
| POST /skills/toggle/:id | 启停 |
| GET /skills/content/:id | 正文预览 |
| POST /skills/delete/:id | 删除（内置技能不可删） |

### 启停的真实实现

PI 只看目录里有什么，没有 "enabled" 标志位。所以**停用 = 把目录移出技能目录**：

```text
{agent-dir}/skills/{name}/        ← 启用
{agent-dir}/skills.disabled/{name}/  ← 停用
```

在 `settings` KV 里记一份启停状态是为了界面渲染（不必每次列两个目录），
但真相是文件在哪个目录。只改 KV 不移动目录会得到一个「界面显示已停用、
PI 照常加载」的假状态。

`/reload` 生效：PI 在 `switch_session` 或下次 spawn 时重扫目录。
WorkBaby 停用后立刻通知 PI 重扫（`prompt` 前的 `get_state` 足够触发懒加载）。

## 内置技能

| 技能 | 用途 |
|---|---|
| office-docs | Word / Excel / PPT / PDF 的读取与产物套路 |
| create-skill | 教用户怎么写自己的 SKILL.md |
| frontend-design | 生成页面时的设计约束 |
| deep-research | 结构化调研：大纲确认 → 逐项搜索 → 交叉验证 → 带引用报告 |
| translation | 中英互译：判用途定风格、术语表全程一致、双语对照交付 |
| resume | 简历按 JD 定制：不编造事实、原文件只读、产出写 resume-output/ |
| travel-plan | 旅行规划：每日行程 / 预算表 / 打包清单三件套 |
| writing | 职场写作与润色：周报 / 邮件 / 通知 / 总结骨架，去 AI 味 |
| prompt-craft | 提示词打磨：角色任务约束格式四件套 + 小样测试循环 |

## 不变量

| 不变量 | 锁死原因 |
|---|---|
| 技能目录 = PI 的技能目录 | 两处各存一份会让「界面里改了但助手行为没变」无法归因 |
| 内置技能必须先落盘再登记 | PI 与模型按真实路径读正文，embed 虚拟路径不存在 |
| 停用必须移动目录 | PI 没有 enabled 标志位，只改 KV 是假状态 |
| 同名导入追加序号而不覆盖 | 覆盖会静默毁掉用户已经改好的技能 |
| 一个坏技能只跳过它自己 | 手写 frontmatter 出错是常态，不许让整个助手起不来 |
| 导入路径经 SafeJoin | zip 与目录导入都是外部输入，`..` 穿越会写出技能目录 |

## 取舍

不做技能市场、不做热插拔执行体。Skill 保持纯文本。

「能不能自己写」不交给模型兜底：内置的 `create-skill` 只是提示词，
技能目录又在工作目录之外，模型没有理由能写进去。因此**新建 / 导入 / 删除必须在
设置页里能点完**，这是一条独立于模型的路径。

代价：技能的真实位置在 PI 的目录里，用户若用 PI CLI 直接加技能，
WorkBaby 的 KV 状态表会短暂不同步。缓解手段是每次列表都以扫描磁盘为准，
KV 只作显示缓存。
