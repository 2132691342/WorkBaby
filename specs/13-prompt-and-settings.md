# 13 · 系统提示与设置

## 定位

系统提示是助手「像个人」的唯一来源：它决定了助手是先看清再动手，
还是一股脑乱干。全部由**工具集驱动**生成——换工具时提示词自动跟着变，
不需要在两个地方维护同一份说明。

## 组装顺序

`service.BuildSystem(env, tools, workspace)`：

```text
persona           固定人设 + 四条工作原则 + 一组输出风格约束
## 可用工具        每个工具一行：名称 + PromptSnippet
## 工具使用准则     各工具的 PromptGuidelines + 兜底准则
## 可用技能         启用技能的 name + description（渐进式披露）
当前工作目录       绝对路径
<project_context>  工作目录下的 AGENTS.md / CLAUDE.md 全文（若有）
```

## persona

**工作原则（四条）**：先看清再动手、说人话、一次做一件事、不编造。
这是写给「不懂命令行的办公人员」的人设，不是通用助手人设。

**输出风格（必须遵守）**：不用 emoji、不用客套开场与套话收尾、
不用「首先/其次/最后」堆段落、不确定就直说不确定。

这一组是产品约束：没有它，模型的输出会混入 emoji 与 AI 腔排版。
规则只存在于提示词里，删掉不会有编译错误，评审提示词改动时必须逐条过。

## 工具驱动

| 工具提供 | 落到哪 |
|---|---|
| `PromptSnippet()` | 「可用工具」清单里的一行 |
| `PromptGuidelines()` | 「工具使用准则」里的条目 |

还有一条**兜底准则**：当工具集里没有 `ls` / `find` / `grep` 但有 `powershell` 时，
自动补一句「用 powershell 做目录浏览与搜索」——
用户把浏览工具停用之后，模型不至于突然不会找文件。

## 项目上下文

工作目录下存在 `AGENTS.md` / `AGENTS.MD` / `CLAUDE.md` 时全文注入
`<project_context>`。这是让用户给自己定规矩的入口：
写了「本项目报告一律用中文」，助手就会照做。

## 设置键位

运行时配置走 `settings` KV 表，键名集中在 `domain/settings.go`：

| 键 | 缺省 | 作用 |
|---|---|---|
| `default_provider` | 空 | 默认模型服务 id |
| `default_model` | 空 | 默认模型名 |
| `permission` | `ask` | 执行方式：ask / auto_edit / yolo |
| `context_reserve_tokens` | 空 | 给模型输出留的 token 余量；空 = 跟随该模型的输出预算 |
| `context_window` | 空 | 全局上下文窗口覆写（空 = 用能力目录值） |
| `disabled_tools` | 空 | 逗号分隔的停用工具名单 |
| `disabled_skills` | 空 | 逗号分隔的停用技能名单（启动时套用） |
| `theme` | `light` | 主题：light / dark |
| `font_size` | `md` | 正文字号刻度 |
| `font_family` | `system` | 界面字体族（`system` = 跟随系统默认） |
| `show_thinking` | `true` | 是否展示思考过程 |
| `send_on_enter` | `true` | 回车发送（false = Ctrl+Enter 发送） |
| `minimize_to_tray` | `true` | 关闭按钮收进托盘；`false` = 点关闭就退出进程 |
| `density` | `comfortable` | 信息密度 |
| `workspace` | 用户主目录 | 默认工作目录 |
| `max_turns` | 空（内置 64） | 单次 run 的轮数上限，失控护栏 |
| `tool_parallel` | 空（内置 4） | 并发工具上限 |
| `stream_idle_seconds` | 空（内置 300） | 上游空闲看门狗阈值，夹取 5 秒 ~ 30 分钟 |

配置文件（`config.yaml`）只放**不适合进数据库**的东西：`master_key`、`workspace`、`log_level`。
监听端口是随机回环，不落配置。

三档执行方式的完整语义与审批闸门见 [`05-exec-policy.md`](05-exec-policy.md)；
档位按会话存（`sessions.permission`），前端在输入框的「规矩」chip 里切换。

## 取舍

- **提示词由工具集驱动**：换工具时说明自动跟着变，不需要两处维护；代价是模型看到的准则
  依赖每个工具的 `PromptSnippet` 写得准，工具作者漏写等于模型看不到这个能力。
- **人设与输出风格只存在于字符串里**：删掉不会有编译错误，改坏也没有测试能拦住。
  代价是评审提示词改动必须逐条人工过，靠规范而不是靠工具。
- **设置全部走 KV 表**：加一个开关不用改表结构；代价是键名与取值没有类型约束，
  写错值只有到使用时才暴露（所以取值都集中在 `domain/settings.go` 一处声明）。

## 约束

- 系统提示里不出现任何用户目录之外的内容
- 工具清单为空时显式写 `(none)`，不要留空让模型自己猜
- 人设文案改动会直接影响助手行为，改之前先想清楚有没有更好的说法
