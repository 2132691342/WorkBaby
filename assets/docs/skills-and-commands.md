# Skill 与自定义命令

Skill 与自定义命令是 WorkBaby 让 AI 「按你的方式工作」的两种核心扩展机制。

## 1. Skill（方法论注入）

Skill 是一份 Markdown 文件，写着「遇到这种情况应该这样思考 / 这样操作」。
命中时，正文会作为方法论注入 system prompt，模型据此调整策略。

### 1.1 文件位置（覆盖顺序：工作区 > 用户级 > 内置）

| 来源 | 路径 | 用途 |
|---|---|---|
| 工作区级 | `<workspace>/.workbaby/skills/<name>/SKILL.md` | 项目专属方法论（随仓库分发）|
| 用户级 | `%APPDATA%/WorkBaby/skills/<name>/SKILL.md` | 跨项目方法论（自己的 dotfiles）|
| 内置 | 应用打包内 | 通用方法论（review / refactor / test …）|

### 1.2 文件结构

```markdown
---
name: code-review
description: 对代码改动做严格 review，输出风险点清单与改进建议。
---

# Code Review Skill

## 输入
- 改动文件列表（`git diff --name-only`）
- 改动内容（`git diff`）

## 流程
1. 按文件风险等级排序：配置 > 网络 > 持久化 > 业务逻辑 > UI
2. 每个文件：检查破坏性变更 / 错误处理 / 边界条件 / 测试覆盖
3. 输出：风险点（严重/中/低）+ 改进建议
```

`name` 与 `description` 是必需的 frontmatter 字段（与 PI SkillSpec 兼容）。

### 1.3 命中方式

- 模型在 system 段看到 `<available_skills>` 列表，主动判断当前任务该用哪个 skill
- 用户可在输入框 `/skill:name` 显式调用（参数跟在后面）
- 内置 Skill 的 `disable-model-invocation: true` 仅允许显式调用，不让模型自动选用

### 1.4 可选脚本

`<name>/scripts/` 下的 `.py` 文件会被识别为可执行脚本。
模型调用 `run_skill_script(skill, script, args)` 即可执行（走 exec 同套沙箱）。

## 2. 自定义命令（斜杠命令）

自定义命令是预制的提示词模板。`/` 面板里点选即可「一键发送」到输入框（可继续修改）。

### 2.1 命令文件位置

| 来源 | 路径 | 优先级 |
|---|---|---|
| 工作区级 | `<workspace>/.workbaby/commands/<name>.md` | 高 |
| 用户级 | `%APPDATA%/WorkBaby/commands/<name>.md` | 中 |
| 设置页 | UI 内联编辑 | 低（同名时工作区 > 用户级 > 设置页）|

### 2.2 文件结构

```markdown
---
description: 把这次的 bug 复现步骤转成 issue 正文
argument-hint: <复现步骤或报错堆栈>
---

# 复现步骤

## 描述
$ARGUMENTS

## 期望
正常运行

## 实际
报错

## 环境
- WorkBaby 版本：
- 系统：Windows 11
```

### 2.3 占位符

| 占位 | 替换 |
|---|---|
| `$ARGUMENTS` | 用户在面板输入框填写的全部文本 |
| `$1` `$2` … | 空格分隔的第 N 个 token |

支持 `$1` 占位适合「固定格式的填充」（commit message / changelog / PR 模板等）。

### 2.4 内置命令

无需文件，WorkBaby 内置的常用命令（`/new` `/clear` `/compact` `/context` `/goal` …）在 `/` 面板里始终可见。
文件命令可与内置名同名：文件优先（覆盖内置），但 UI 上会标注「覆盖」便于识别。

## 3. Skill vs 命令：何时用哪个

| 场景 | 选 Skill | 选命令 |
|---|---|---|
| 「每次都让 AI 这样思考」| ✅ | — |
| 「AI 主动判断用不用」| ✅ | — |
| 「用户主动触发，固定提示词」| — | ✅ |
| 「可执行脚本（需要 Python 环境）| ✅（脚本）| — |
| 「AI 自己调用工具做某事」| — | ✅（用 `delegate_task` 委派子 Agent）|

## 4. 故障排查

| 症状 | 检查 |
|---|---|
| Skill 没出现在模型工具列表 | `disable-model-invocation: true`？`name` 与 `description` 字段是否齐全？|
| 命令没出现在 `/` 面板 | 文件名是否符合 `^[a-z][a-z0-9-]{0,63}$`？目录权限？|
| Skill 脚本执行失败 | `%APPDATA%/WorkBaby/logs/` 的 exec 行；脚本是否 shebang 正确？|