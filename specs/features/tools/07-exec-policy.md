# 07 · 命令执行与风险裁决

## 1. 三层拦截

`exec` 工具的安全由三层叠加保证：

| 层 | 机制 | 拦截什么 |
|---|---|---|
| 1 | 白名单（basename 精确匹配） | 不在白名单的二进制 |
| 2 | 危险模式正则（匹配整条 command + args） | 破坏性命令（`rm -rf`、`del /s`、`format`、`shutdown`…） |
| 3 | 命令级风险裁决 + 审批门 | 白名单外 / 危险命令需人工确认 |

空白名单会让每条命令都弹审批，长任务在人工响应前就被墙钟预算砍断——**非空白名单是「Agent 能干活」的前提**。

## 2. 白名单

`DefaultExecPolicy()` 内置常用工具链：

| 类别 | 二进制 |
|---|---|
| Windows shell | `cmd` `powershell` `pwsh` |
| 版本管理与构建 | `git` `go` `node` `npm` `npx` `pnpm` `yarn` `python` `python3` `py` `uv` `uvx` `dotnet` `java` `javac` `mvn` `gradle` `make` |
| 网络与归档 | `curl` `wget` `tar` `unzip` |
| cmd 内建命令 | `dir` `type` `echo` `copy` `move` `mkdir` `rmdir` `find` `findstr` `where` `set` |

运行时白名单可被 `system_settings` 的 `SettingKeyExecWhitelist` 整体覆盖（设置页可编辑）。

## 3. 危险模式

13 条正则（`DefaultExecPolicy.DeniedPatterns`），命中即 `irreversible`：

```
rm -rf / -fr / -r      del /f /s        del /s          rd /s /q
rmdir /s               format <盘符>     mkfs            diskpart
cleanmgr               cipher /w        reg delete      shutdown
taskkill /f
```

`Compile()` 预编译正则（`Classify` 是热路径，逐次 `MustCompile` 是浪费）；未编译时回退即时编译，语义不变。

## 4. 命令级裁决（Classify）

`ExecPolicy.Classify(command)` 返回 `(ok, risk)`，`ExecTool.ClassifyArgs` 把它接到护栏链：

| 情况 | 裁决 | 后续 |
|---|---|---|
| 白名单内且未命中危险正则 | `ok=true` | 免审直行 |
| 命中危险正则 | `risk=irreversible` | 必须人工批准（不可逆永不免审） |
| 白名单外 | `risk=needs_approval` | 走审批门 |
| 空命令 | `risk=""` | 直接拒绝（不进审批） |

**命令级而非工具级**：`exec` 这个工具本身风险高，但 `git status` 与 `git push --force` 应当区别对待。
`ClassifyArgs` 让审批卡展示真实命令而非工具名。

## 5. 执行细节

### 参数数组，绝不拼 shell

```go
execReq{Command: "git", Args: []string{"status", "--short"}}
```

命令与参数分离传递，不经过 shell 拼接——从根本上消除注入面。

### Windows 平台解析

`.cmd` / `.bat` 外壳（npm / npx 等）与 cmd 内建命令（dir / type）必须包一层 `cmd /c` 才能 `CreateProcess`。
白名单校验仍针对原始 command，包 shell 只是执行细节。

### 内置运行时优先

`resolveWithBuiltin(name, dirs)`：先在**内置运行时 bin 目录**查找，未命中再回落进程 PATH。
解析过程不改动进程全局 PATH（并发 exec 下是数据竞争）。

### 工作目录

| 输入 | 行为 |
|---|---|
| LLM 显式传 `cwd` | 越界校验后使用（含 `..` → 4007；越出工作区 → 4008） |
| 未传 | 跟随会话工作区根（解析出的根必须真实存在才生效，否则维持进程当前目录） |

`cmd.Dir` 落定后**显式注入 `PWD=` 到子进程 env**——让 `python -c "import os; print(os.getcwd())"`
这类自报路径与父命令一致，避免「沙箱解析到 X、子进程实际落在 Y」的认知断裂。
实际目录同时写入 `ToolResult.Meta["cwd"]`，前端工具卡展示为徽标。

### 输出限流

`cappedWriter` 合并 stdout / stderr 做**双端保留**：头 120KB + 尾 80KB，中间丢弃并计数。

| 为什么双端 | 只留头会丢掉错误栈与最终统计（最常要看的）；只留尾会丢掉命令回显与首屏上下文 |
|---|---|
| 为什么限流 | `CombinedOutput` 无上限，`cat 大文件` 这类命令会把全量输出读进内存造成 OOM |
| 丢弃计数 | 丢掉的字节仍计入返回值，保证子进程不因管道写满而阻塞 |

### 输出编码归一化

子进程 stdout 经常不是 UTF-8（Windows PowerShell 默认 GBK/cp936 直出，UTF-8 解析会乱码）。
`decodeConsoleOutput` 按 BOM → UTF-8 校验 → 启发式 GBK 的顺序归一化：

| 顺序 | 判据 |
|---|---|
| 1 | 字节流以 BOM 开头 → 按 BOM 类型解码（UTF-8 / UTF-16LE / UTF-16BE） |
| 2 | 无 BOM 且 UTF-8 校验通过 → 直通 |
| 3 | UTF-8 校验失败 → 按 GBK 解（CJK 为主的容错） |

模型与事件流都看到一致的 UTF-8 文本。

### 其他

| 项 | 处理 |
|---|---|
| 超时 | 策略默认 5 分钟，可由入参 `timeout`（毫秒）覆盖 |
| 窗口 | `SysProcAttr{HideWindow: true}`，不弹控制台 |
| 退出码 | 写入 `Meta["exitCode"]`，前端可据此区分失败原因 |

## 6. 技能脚本

`run_skill_script` 与 `exec` 共用白名单与危险模式判定，但解释器固定映射（不由模型指定），2 分钟超时。

## 7. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 白名单 + 危险正则 + 命令级裁决 | 三层拦截；免审命令直行不打断长任务 | 需维护白名单；首次使用有学习成本 |
| 参数数组（无 shell 拼接） | 消除注入面 | 管道 / 重定向需显式经 `cmd /c` 或 `powershell -Command` |
| 内置运行时优先于系统 PATH | 工具开箱可用，版本可控 | 用户想用系统版本时需调整设置 |
| 输出双端限流 | 防 OOM 且保留最常看的两端 | 中间内容丢失（有明确标记） |
| 输出编码启发式归一化 | 中文不再乱码 | 极少数非 GBK 的本地编码可能误判 |
| cwd 越界硬拒绝 | 防 LLM 把过程脚本写到用户目录外 | 需要跨目录操作时须先绑定工作区 |
