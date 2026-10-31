# 错误码 · ERROR-CODES

码值为数字字符串，随 `Resp.code` 下发；`"0"` 为成功。段位见 `AGENTS.md` §2.4。
处置列是前端应展示的动作提示；产生方列是 raise 该码的 crate（grep 实际使用处）。

表内只收有真实使用处的码：常量定义了但零使用的码不进表；
新增错误码先把本表补上，再写代码。

## 1000–1999 通用 / 文件 / 路径

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 1001 | 参数缺失或非法 | wb-pkg / wb-tools / wb-git / wb-service / wb-api | 检查表单 |
| 1002 | 文件读写失败 | wb-pkg / wb-tools / wb-kernel / wb-features / wb-git / wb-service / wb-api | 查看文件是否被占用 |
| 1003 | 路径越界（逃出允许根） | wb-pkg / wb-git | 换工作目录内路径 |
| 1004 | 路径不存在 | wb-tools / wb-git / wb-service | 确认路径 |
| 1017 | git 命令失败（通用） | wb-git | 看详情里的 stderr |
| 1020 | git 提交失败 | wb-git | 检查暂存区与用户名配置 |
| 1021 | 合并/变基冲突未解决 | wb-git | 先解决冲突 |
| 1024 | 远端领先（push 被拒） | wb-git | 先 pull |
| 1030 | 压缩包解析失败 | wb-service | 检查 zip 结构 |

## 2000–2999 配置 / 持久化 / DB

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 2001 | 配置文件损坏（JSON 解析失败） | wb-provider / wb-kernel | 用详情定位行，修复或重置 |
| 2002 | 数据库打开/迁移失败 | wb-repo / wb-features / wb-service | 查看日志 |
| 2004 | 数据目录不可写 | wb-repo / wb-features / wb-kernel / wb-api | 检查磁盘与权限 |

## 3000–3999 Provider / 模型

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 3001 | provider 不存在 | wb-provider / wb-service | 检查配置 |
| 3002 | 模型不存在 | wb-provider / wb-service | 刷新模型清单 |
| 3003 | API key 缺失 | wb-service | 填密钥 |
| 3005 | baseUrl 非法 / wire api 不支持（当前仅 openai-completions） | wb-provider / wb-service | 检查地址；核对 wire api |
| 3109 | 上游响应解析失败 | wb-service | 确认该地址返回兼容 /models 的响应 |
| 3111 | 名称重复 | wb-service | 换名 |
| 3200 | 模型连通测试失败 | wb-provider / wb-service | 看详情（401/404/网络） |

## 5000–5999 会话运行

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 5001 | 会话忙（已有 run 在跑） | wb-service | 等待或先停止 |
| 5002 | 会话不存在 | wb-service | 刷新列表 |
| 5003 | 会话文件被占用/删除失败 | wb-api | 等运行结束再删 |
| 5004 | 队列已满 | wb-service | 稍后再发 |
| 5005 | 内核提问已过期 | wb-features / wb-api | 重新发起 |
| 5006 | 压缩失败 | wb-kernel | 重试或减少保留量 |
| 5100 | 内核未就绪（未配模型） | wb-service | 先配置模型 |

## 7000–7999 工具执行与外部进程

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 7001 | 后台任务失败 / 外部调用超时（功能后台任务、LSP 响应、MCP 调用 60s、克隆 300s） | wb-features / wb-service | 看日志定位失败任务 |
| 7002 | 命令被取消 | wb-tools / wb-features / wb-service | — |
| 7003 | 工具不存在/已停用 / 外部进程启动失败 | wb-tools / wb-features / wb-service | 检查工具开关 |
| 7004 | 工具参数校验失败 | wb-features / wb-kernel / wb-service | 看详情 schema 提示 |
| 7100 | LSP server 启动失败 | wb-features | 确认语言服务器已安装并在 PATH |
| 7101 | LSP server 返回错误响应 | wb-features | 看详情里的 server 消息 |
| 7102 | 语言服务器未安装 | wb-features | 装上对应 server（或关掉 lsp 功能） |
| 7103 | 没有匹配该文件类型的语言服务器 | wb-features | 换文件类型或配自定义 server |
| 7301 | 记忆条目不存在 | wb-features | 刷新记忆清单后再操作 |

## 8000–8999 Skill

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 8001 | SKILL.md 解析失败 | wb-skills / wb-service | 检查 frontmatter |
| 8002 | description 缺失或超长 | wb-service | 补全（≤1024 字符） |
| 8003 | 技能名非法（小写字母数字连字符） | wb-service | 改名 |
| 8004 | 技能名冲突 | wb-service | 内置/用户同名，改目录名 |
| 8005 | 内置技能不可删改 | wb-service | — |

## 9100–9199 MCP

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 9101 | server 配置非法 | wb-domain / wb-mcp / wb-service | 检查 mcp.json |
| 9102 | server 启动失败/超时 | wb-mcp / wb-service | 确认命令可用；看 mcp 日志 |
| 9103 | 工具调用失败 | wb-mcp | 看 server 返回 |

## 9700–9799 内核

| 码 | 含义 | 产生方 | 处置 |
|---|---|---|---|
| 9701 | 会话文件损坏（回放失败） | wb-kernel | 用详情定位坏行；可从上一条目继续 |
| 9703 | 模型流中断（不可重试） | wb-kernel | 重发消息 |
| 9704 | 子代理启动失败 | wb-kernel | 看详情 |
