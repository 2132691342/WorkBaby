# 22 · 插件（PI 扩展与包）

## 定位

PI 的能力靠扩展往前长：一个 `.ts` 文件就能注册新工具、新斜杠命令、改系统提示。
插件面板负责把这件事变成「看得见、装得上、关得掉」，不必记 `pi install` 的写法。

它只管**清单与文件**，不解释扩展代码本身——扩展怎么写、什么时候被调用，都是 PI 的事。

## 设计

| 决策 | 选择 | 为什么 | 代价 |
|---|---|---|---|
| 启用清单 | `settings.json` 的 `extensions` | PI 启动期只读这一份；搬目录等于把已装插件在磁盘上反复挪位置 | 改完必须重启 PI 才生效 |
| 条目来源 | 目录扫描 ∪ 清单登记 | 只扫目录看不到登记在别处的路径；只看清单看不到刚丢进去还没启用的文件 | 两边都要去重（按绝对路径、大小写不敏感） |
| 装包 | 直接跑 `pi install` / `pi remove` | 包的解析、版本、git ref 归 PI 自己的包管理；重写一份就是永久追版本 | 依赖 PI 可执行文件（没有则 9709） |
| 进度 | `chat:setup` 走全局 SSE 通道 | npm 拉包几十秒，串在一个 HTTP 请求里既占连接又看不到中间失败 | 前端要轮询列表确认落地 |
| 内置闸门 | `workbaby.ts` 标 `builtin`，停用 / 删除报 9717 | 它实现了审批卡与记忆写入工具；停掉后界面一切正常，只是再也不弹审批 | 用户想换自己的闸门要先改代码 |
| 写入方式 | 只改自己掌管的顶层键 + 嵌套对象按字段合并 | 见下节不变量 | 结构体里加字段要同步 `settingsOwnedKeys` |

### settings.json 的写入纪律

`SettingsFile` 只覆盖它列的顶层键（`defaultProvider` / `defaultModel` /
`defaultThinkingLevel` / `defaultTools` / `compaction` / `extensions`）。
写之前先摘下这几个键的旧值，再把结构体能序列化出的值写回去，其余顶层键原样保留。
嵌套对象（目前只有 `compaction`）按字段合并：我们只改 `reserveTokens` / `keepRecentTokens`，
里面的 `enabled` 归 PI 管，不能被写丢。

原因很直接：PI 自己也往这个文件写东西（`packages` / `cacheWarming` / `codemode` / `prompts`…），
用户还会用 `pi install` 往里加包。整体重写会把它们一次抹掉，而症状是
「我装过的插件怎么没了」，不是「设置写失败」。

`packages` **不在**掌管键里：它归 PI 的包管理，我们只在装 / 卸后读回展示。

## 契约

| 方法 | 路径 | 出参 |
|---|---|---|
| GET | `/extensions` | `ExtensionListRESP{dir, settings_path, entries[], packages[]}` |
| POST | `/extensions/toggle` | `{path, enabled}` → `true` |
| POST | `/extensions/import` | `{paths}` → `{imported, skipped[]}` |
| POST | `/extensions/delete` | `{path}` → `true` |
| POST | `/extensions/install` | `{source}` → `true`（后台执行，进度走 `chat:setup`） |
| POST | `/extensions/remove` | `{source}` → `true` |

`entries[].kind` 取 `file` / `dir` / `pattern`（通配登记，界面只读）/ `missing`（登记了但磁盘上没有）。
`builtin=true` 的条目不给开关与删除按钮。

来源写法：`npm:@scope/pkg@1.0`、`git:github.com/owner/repo@v1`、`./本地目录`、裸包名。

## 关键流程

1. 打开设置 → 插件：读 `settings.json` 拿到登记项，扫 `extensions/` 拿到目录项，按路径去重合并
2. 启用 / 停用：改写 `extensions` 数组 → 落盘 → `RestartAll`（扩展是进程启动期加载的）
3. 装包：`POST /extensions/install` 立刻返回 → `pi install <source>` 逐行推 `chat:setup` →
   前端每 1.5s 重读列表，直到 `packages` 里出现该来源（或 5 分钟超时）
4. 装完自动重启 PI；卸包同理

## 不变量

| 不变量 | 锁死原因 |
|---|---|
| 写入 `settings.json` 只动自己掌管的键 | 整体重写会抹掉 PI 与用户的配置（含已装的包） |
| 启停只改清单、不搬文件 | 搬目录会让「导入过的插件」在磁盘上漂移，用户找不到它 |
| 内置审批闸门不可停 | 停掉后审批卡永久消失，而界面上没有任何提示 |
| 包操作串行（同一把锁） | 两次 `pi install` 并发会写坏同一个包目录 |
| 装 / 卸后必须重启 PI | 扩展与包都在进程启动期加载，文件改了不重启等于没改 |
| 路径目标走请求体 | Windows 绝对路径带盘符与斜杠，拼不进路径参数 |

## 取舍

- **不做插件市场**：列表里只有「你已经有的」和「你知道来源的」。做市场就要处理审核、
  版本推荐、签名与信任提示，那是另一个产品的工作量
- **不解析扩展代码**：界面不读扩展导出了哪些工具、注册了哪些命令。它能做到，但结论会随
  扩展实现变化，且误判的代价是「界面说得和实际不一致」。要知道有哪些工具去看设置 → 工具
  （那份清单是 PI 报上来的）
- **重启而非热重载**：PI 没有热重载协议面。代价是切一次开关会中断同工作区的进程，
  但 `agent_settled` 之后进程本来就是闲置的，实际影响只在「正在跑的时候改插件」
- **不做依赖预检**：包缺 npm 依赖时由 PI 自己在运行期报错。预检要把包的依赖树解析一遍，
  等于重新实现一次 npm 的一部分
