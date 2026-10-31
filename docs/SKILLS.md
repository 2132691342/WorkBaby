# 技能体系 · SKILLS

技能是「一份可复用的工作方法」：一份带 frontmatter 的 Markdown，
告诉模型在什么场景下该按什么步骤做。

技能不是工具——它是提示词注入 + 用 read 加载正文。
做成工具会让模型多一层无谓的调用往返。

## SKILL.md 格式

```markdown
---
name: tdd
description: |
  用测试驱动开发推进功能修改。先写失败测试，再写实现。
description_zh: 用测试驱动开发推进功能修改
version: 1.0.0
allowed-tools: Read,Write,Bash
---

# 正文

方法步骤、命令、模板……
```

### frontmatter 字段

| 字段 | 必填 | 说明 |
|---|---|---|
| `name` | 否（缺省回退目录名，仍须过命名规则） | 技能名即身份；小写字母数字与单连字符，≤64 字符 |
| `description` | 是 | 何时用这个技能；≤ 1024 字符（会进系统提示词） |
| `description_zh` / `description_en` | 否 | 分语言描述，界面展示用 |
| `version` | 否 | 版本号 |
| `license` / `homepage` / `author` | 否 | 元信息 |
| `allowed-tools` | 否 | 建议工具名 |
| `triggers` / `keywords` | 否 | 触发词与关键词 |
| `metadata` | 否 | 自由嵌套，原样保留 |
| `disable-model-invocation` | 否 | true 则只允许 `/skill:名` 显式调用 |

未知键不报错，原样保留在兜底字段里。

**description 用块标量**（`description: |`）写多行——单行写不下完整触发条件。

## 三区发现

| 区 | 位置 | 扫描顺序 | 可否删改 |
|---|---|---|---|
| 内置包 | 编译期打进二进制，解压到 `{数据目录}\skills\builtin` | 1 | 否（只读） |
| 用户区 | `{数据目录}\skills\user\{技能名}\SKILL.md` | 2 | 可 |
| 工作区 | `{工作区}\.workbaby\skills\{技能名}\SKILL.md` | 3 | 可 |

**同名后扫的覆盖先扫的**：优先级 工作区 > 用户 > 内置。
这是刻意设计——工作区技能用于就地试验与项目级覆盖，用户技能跨项目生效，
内置包是兜底基座；上层覆盖下层，试验成熟了挪进用户区即可。

## 注册表

`wb-skills::SkillRegistry` 扫描三个区，建立 `技能名 → 条目` 的映射。
条目含 frontmatter、来源标记与位置路径。

扫描时的跳过规则：目录不存在、没有 `SKILL.md`、frontmatter 解析失败、
description 缺失或超长——**坏技能只跳过自己，不挡整表**。
一个技能写坏了不该让所有技能都不可用。

开关状态存在 `wb-repo` 的 `skill_toggle` 表（**未记录 = 开**）。
内置技能的开关可用，但不可删除也不可改名——内置包是应用的一部分。

## 注入系统提示词

启用的技能在组装系统提示词时注入：

```xml
<available_skills>
<skill><name>tdd</name><description>…</description><location>…</location></skill>
</available_skills>
```

只注入名称与描述，不注入正文——正文按需用 read 加载。
9 个技能全注入也只有几十行，正文全注入会占掉大量上下文。

`disable-model-invocation` 的技能不进这段注入，只能通过 `/skill:名` 显式调用。

## 斜杠命令

启用的技能对应 `/skill:技能名` 命令，走 `/commands` 端点列出。
显式调用会把正文直接送进上下文。

## 内置技能包

9 个技能随二进制分发，覆盖几类场景：

| 类别 | 技能 |
|---|---|
| 前端与设计 | 前端开发、全栈开发、canvas-design、Impeccable（前端设计工具集） |
| 文档处理 | markitdown-skill、md-to-pdf-cjk |
| 工程质量 | skill-creator、skill-scanner、skill-vetter |

解压带**指纹哨兵**：每次启动递归统计包内文件数与总字节数，
写入 `{builtin}\.pack_stamp`；与包不符即清空重解压——
升级与技能删减后旧文件不残留，解压目录始终与打进二进制的包一致。

内置技能里的脚本随包分发（如文档转换的 shell 脚本），执行时按技能自身的说明调用。

## 导入与创建

| 操作 | 端点 | 说明 |
|---|---|---|
| 从目录导入 | `POST /skills/import` | 源目录必须含 SKILL.md，拷入用户区 |
| 从 zip 导入 | `POST /skills/import-zip` | base64 传字节 |
| 新建 | `POST /skills/create` | 写用户区 SKILL.md |
| 删除 | `POST /skills/delete/{id}` | 内置技能拒绝 |
| 读正文 | `GET /skills/content/{id}` | 给界面抽屉用 |

导入与创建后会重扫注册表——三区发现是启动时扫的，运行时新增文件要触发重扫。

## 校验

| 规则 | 不通过的后果 |
|---|---|
| 技能名合法（小写字母数字 + 单连字符，≤64 字符） | 跳过该技能 |
| description 非空且 ≤ 1024 字符 | 跳过该技能（超长会撑爆系统提示词） |
| 目录含 SKILL.md | 跳过并记录原因 |

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 三区发现而非单一目录 | 内置随包分发，用户与工作区各管一段 | 优先级规则要讲清 |
| 内置只读 | 内置包是应用的一部分，改了就对不上版本 | 用户不能微调内置技能 |
| 只注入名称与描述 | 正文按需加载，省上下文 | 模型得自己决定要不要 read |
| 技能不产生工具 | 避免无谓的调用往返 | 技能无法直接执行动作，只能指导模型 |
| description 上限 1024 | 它进系统提示词，长了挤占上下文 | 复杂技能要写得精炼 |
| 坏技能跳过而非整表失败 | 一个写坏的技能不该让全部不可用 | 出错只体现在日志里 |
