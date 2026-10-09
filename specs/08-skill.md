# 08 · Skill 系统

## 定位

Skill = 一段按需注入的领域知识（prompt 素材），不是可执行插件。
新手小白的「秘籍」：让助手在特定场景按固定套路干活。
纯文本，**安全面为零**——不产生任何执行行为，新手也能自己写。

## 文件格式

```
assets/skills/{name}/SKILL.md                    # 内置（embed），启动时落到磁盘
%APPDATA%/WorkBaby/builtin-skills/{name}/SKILL.md  # 内置落盘位置（真路径）
%APPDATA%/WorkBaby/skills/{name}/SKILL.md        # 用户全局
{workspace}/.workbaby/skills/                    # 工作区级
```

内置技能虽然在源码里是 embed 资源，启动时必须先**写一份到磁盘**再登记：
模型是按路径去 `read` 正文的，虚拟路径在 `read` 眼里不存在。
落盘目录与用户技能目录分开，免得被「全局技能」扫描重复登记成两条；
写失败时退化为「正文只留在内存」，`/技能名` 这类显式调用仍然可用。

```markdown
---
name: weekly-report
when_to_use:
  - 周报
  - 写周报
description: 按团队模板把本周工作整理成周报。
---
（正文：给模型看的操作指引，Markdown）
```

`name`（小写字母 / 数字 / 连字符，≤64）与 `description` 必填；`when_to_use`
是可选触发词列表（标量也接受），会随清单一起注入系统提示。解析失败只跳过这一个技能并记 warn，
**一个坏技能不许让整个助手起不来**。

## 三级来源与优先级

| source | 目录 | 同名冲突 |
|---|---|---|
| builtin | embed | 低 |
| global | 数据目录 | 中 |
| workspace | 工作目录 | 高 |

`Get(name)` 返回最高优先级的那份；`List()` 全部列出并带 `source` 字段，
让用户看得见「这个技能是从哪来的」。切换工作目录时先摘掉上一个工作区的技能再装入
新的（`DropSource` + `LoadWorkspace`）——不重载的话切完目录列表还是旧的那套。

## 注入方式（渐进式披露）

系统提示只注入启用技能的「name + description + 触发词」清单，一行一条；
**正文不常驻**——模型判断该用某个技能时，自己调 `read` 把它当文件读进来。
技能一多就把上下文撑爆，渐进式披露是唯一可行的做法。

用户也可以在输入框打 `/` 从菜单选技能，或直接发 `/skill:名字 需求说明`：正文由 service
展开后拼成一条用户消息发出去（需求说明附在展开正文之后），与手打的消息走同一条落库链路
（`entries` 里就是一条普通 user 条目），只是发送前多一步展开。

## 管理

| API | 说明 |
|---|---|
| GET /skills | 列表（含来源与启停状态） |
| POST /skills | 新建：写 `SKILL.md` 到全局技能目录并登记 |
| POST /skills/import | 从磁盘导入 `{paths[]}`：单个 SKILL.md 或含它的文件夹 |
| POST /skills/import-zip | 导入压缩包 `{filename, data}`（zip 字节 base64）：解压到临时目录后走同一条定位复制链路；条目路径经 SafeJoin 校验防穿越，单文件 64MB / 总量 256MB 双限流 |
| POST /skills/:id/toggle | 启停，落 settings KV，重启后仍生效 |
| GET /skills/:id/content | 正文预览 |
| POST /skills/:id/delete | 删除（内置技能不可删） |

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

## 取舍

不做技能市场、不做热插拔执行体。Skill 保持纯文本，
用户改错了大不了模型照着做错一点，不会出事。

「能不能自己写」不交给模型兜底：内置的 `create-skill` 只是提示词，
技能目录又在工作目录之外，模型没有理由能写进去。
因此**新建 / 导入 / 删除必须在设置页里能点完**，这是一条独立于模型的路径。
