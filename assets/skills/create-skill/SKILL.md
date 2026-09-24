---
name: create-skill
version: 2.0.0
when_to_use:
  - 创建 skill
  - 新建 skill
  - 写一个 skill
  - 编写 skill
  - skill.md 格式
  - create skill
  - 技能怎么写
description: 按 WorkBaby 的 SKILL.md 约定创建、校验并落地一个新技能。当用户想新增/编写技能、修改已有技能、或询问技能格式与触发词写法时使用。
allowed_tools: ["file_read", "file_write", "file_list", "file_edit", "exec"]
---

# 创建 WorkBaby 技能

技能是一段**按需注入**的方法论：触发时才进入上下文，用来约束 Agent 在某类任务上怎么做。
 skills 表是唯一真相源，SKILL.md 只是导入格式。

## 先判断是否值得建技能

| 判断 | 结论 |
|---|---|
| 一次性任务 | 不建，直接对话 |
| 反复出现、每次都要交代同样规矩 | 建 |
| 通用能力（写作/翻译/分析） | 不建，模型本来就会 |

## SKILL.md 格式

```markdown
---
name: my-skill
version: 1.0.0
when_to_use:
  - 触发词一
  - trigger-word
description: 一句话说明这个技能做什么、什么时候用。
allowed_tools: ["file_read", "file_write", "exec"]
---

# 正文标题

正文：分步骤的操作规程。
```

字段规则：

- `name`：小写连字符，与目录名一致，全局唯一。
- `description`：**决定会不会被用到**，写清「做什么 + 何时用」，不写实现方式。
- `when_to_use`：触发词列表，大小写不敏感；写用户真实会说的中文词与英文词。留空则退化为 name 兜底——等于只能靠名字命中，等于死技能。
- `allowed_tools`：可选。技能命中期间允许的工具白名单，能收敛就收敛。
- `version`：可选，默认空。
- 正文不能为空：**只有 frontmatter 的技能会被解析失败并跳过**，表现为「装进去了但一次都触发不了」。

## 正文怎么写才管用

1. 写成**可判定的动作**，不要写「要认真、要专业」这类无法验证的话。
2. 用表格做「场景 → 做法」的快速索引，Agent 先查表再行动。
3. 写清验收：做完后要跑什么命令、读什么文件来证明做完了。
4. 写清失败路径：某步失败后改用什么办法，而不是原样重试。
5. 正文自包含：内置技能只会加载 SKILL.md，**不要链接 `references/*.md` 或 `scripts/*.py`——它们不会被读到**。

## 落地路径

| 来源 | 位置 | 说明 |
|---|---|---|
| 内置（随程序发布） | `assets/skills/{name}/SKILL.md` | 自包含，无附属文件；改代码后重启生效 |
| 自定义 | `%APPDATA%\WorkBaby\skills\{name}\SKILL.md` | 同级 `scripts/` 会被一起入库，`run_skill_script` 可执行 |
| 工作区 | `<工作区>/.workbaby/skills/{name}\SKILL.md` | 绑定该工作区的会话才可见，同名覆盖全局 |

新建后校验：

1. `name` 与目录名一致，且不与已有技能重名。
2. 用真实的一句用户话解去试：`when_to_use` 能不能命中。
3. 走一遍正文流程，确认每步引用的工具都在 `allowed_tools` 里。
4. 内置技能对用户只读（不能改不能删），要迭代就先在自定义目录里写好再迁过去。
