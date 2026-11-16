# 安全说明

面向想了解 WorkBaby 安全边界的人：这里写清威胁模型、已落地的防线、**已知限制**与报告渠道。
接口与字段看 `docs/API.md`，数据落盘看 `docs/DATA-MODEL.md`，日常使用看 `docs/USER-GUIDE.md`。

---

## 1. 威胁模型

WorkBaby 是**单机桌面应用**：后端 HTTP 服务只绑 `127.0.0.1` 的随机端口（`backend/api/server.go` 的 `Start`），
不对外网开放，没有多用户概念。据此划分信任边界：

| 对象 | 信任度 | 防线 |
|---|---|---|
| 本机登录用户 | 可信 | 数据目录在其名下（`%LOCALAPPDATA%\WorkBaby`），密钥对它天然可见 |
| 本机其他进程 / 本机网页 | 不可信 | 启动随机令牌 + 来源与 Host 门禁（见 §2） |
| 模型输出、联网抓回的内容 | 不可信 | 渲染出口统一消毒（XSS 防线） |
| 上游模型服务 | 半可信 | 密钥仅在请求时发出；错误原话脱敏；SSRF 复核约束出站目标 |

**不在威胁模型内**：多用户隔离；与你同账户的恶意进程（它能直接读你的数据目录）；
管理员 / 内核级攻击；静态数据加密（见 §3 第一条）。

门禁的核心是**每次启动随机的令牌**（24 字节熵，`backend/shell/app.go` 的 `newToken`）：
HTTP 走 `X-Desktop-Token` 请求头；只有 SSE 的 `/events` 允许 `?token=` 查询参数（EventSource 带不了自定义头）；
比较用常量时间（`subtle.ConstantTimeCompare`），鉴权失败回 401 不回信封。
来源（Origin）放行 WebView2 的 `wails.localhost` 虚拟域与 `localhost` / `127.0.0.1`；
Host 白名单只认回环主机名（`127.0.0.1` / `localhost` / `::1`，端口任意）——
这是 DNS rebinding 的零成本纵深：rebinding 后 Host 仍是攻击者的域名。

---

## 2. 已落地防线

- **XSS 消毒**：渲染进程唯一的 HTML 出口（Markdown 渲染）统一过 DOMPurify
  （`frontend/src/utils/md.ts` 的 `renderBlocks` → `sanitize`），禁 `script` / `iframe` / `style` / `form`
  与全部事件属性。模型输出与抓回的网页都按不可信内容消毒。
- **SSRF 拨号复核**：出站请求经 `safeDial`（`backend/web/client.go`）——域名解析后**逐 IP** 校验
  内网 / 回环 / 链路本地段，重定向逐跳校验（`CheckRedirect`），不让"重定向进内网"绕过首跳检查。
- **路径边界**：工具路径经 `resolve` 归一（`backend/agent/tool.go`），工作区外一律拒；
  符号链接 / junction 指向区外按真实路径拒绝（`pkg.RealPath`）；会话文件有目录守卫
  （`parser.GuardSession`）。**技能正文只读**：写类工具对技能文件两层判定——入参原文先拦，
  解析后（含真实路径、大小写变体）再拦（`resolveWrite`），相对路径与符号链接绕不过去。
- **密钥掩码**：`/providers`、`/settings`、`/config/text` 一律以掩码流出（前 4 + `••••` + 后 4 或整段 `••••`）；
  `/config/text` 对设置台账走与 `/settings` 同一套掩码，解不开的 JSON 整份不给；
  模型探测的错误原话经 `redactSecret` 脱敏；google 协议密钥走 `x-goog-api-key` 头，不拼进 URL。
- **危险命令确认闸门**：`bash` 命中危险特征（递归删除 / 格式化 / 注册表写入 / 停机重启 / 磁盘擦除）
  先经确认卡（`confirmDestructive`，`backend/agent/tool_shell.go`）；拒绝或没有确认通道**一律不执行**。
  用法与开关见 `docs/USER-GUIDE.md` §6。
- **令牌与来源收紧**：发布版禁用 `WB_DEV_TOKEN`（只有开发态布局接受，见 `docs/DEV.md` §2.4）；
  熵源不可用直接拒启，不退化成可预测令牌；查询参数令牌只允许 `/events` 一条路由。
- **资源上限**：请求体超限即拒；SSE 慢订户摘号断连；`grep` 单文件超 1MB 跳过、命中数截断提示；
  PDF 解压单流 32MB / 整份累计 64MB；命令输出 40k 截断、执行超时上限 600s；一轮最多 24 步工具调用。
  子进程统一隐藏窗口 + 超时杀树（`pkg.RunProc` / `KillTree`）。

这些边界大多有测试看住：鉴权门、路径守卫、密钥掩码、SSRF 拦截等见 `tests/`
（`http_test.go`、`backend_hardening_test.go`、`workspace_test.go`）。

---

## 3. 已知限制

**用之前应该知道的**：

1. **密钥明文存储**：服务商 API Key 明文存 `workbaby.db`（SQLite），联网搜索密钥
   （`web.search_key`）明文存 `workbaby.json` —— **没有做 DPAPI 或其他加密**。
   能读你数据目录的进程或账户就能拿走密钥。缓解：数据目录在当前用户的 `%LOCALAPPDATA%` 下；
   不要在共享账户的机器上使用；怀疑泄漏就去服务商侧轮换密钥，删 `workbaby.db` 等于吊销全部（需重填）。
2. **SSE 令牌走查询参数**：`/events` 的令牌在 URL 里（`?token=`），同机进程的命令行 / 代理日志可能看到它。
   其余路由只认 `X-Desktop-Token` 请求头。拿到令牌的进程可以接事件流、调接口——它等价于"是你"。
3. **代理环境下 SSRF 语义变化**：`HTTP_PROXY` / `HTTPS_PROXY` 生效时连接的是代理，
   `safeDial` 复核的是拨号目标（代理地址）而不是最终目标——企业代理环境下
   "内网地址拦截"的保证不再等价，重定向的逐跳校验语义也随之改变。
4. **危险命令闸门是确认不是沙箱**：特征是子串匹配（不区分大小写），覆盖常见危险写法但拦不住全部；
   确认卡只在命中特征时出现。把 `agent.bash_confirm` 置 `"0"` 关掉后，模型可直接执行任意命令。
5. **消毒只覆盖已知渲染出口**：XSS 防线建在 Markdown 渲染出口上；未来新增 HTML 注入面
   必须同样过 `sanitize`，否则防线对它无效。
6. **会话与日志明文落盘**：`sessions\*.jsonl` 与按天日志不加密。日志里的密钥已脱敏，
   但命令输出、文件路径、对话内容本身对能读目录的人可见。

---

## 4. 报告问题

- 渠道：GitHub Issue —— <https://github.com/2132691342/WorkBaby/issues>（安全问题可以只描述影响面与复现步骤）。
- 报告时**不要**贴密钥原文、未脱敏的配置 / 日志片段；涉及凭据请先自行轮换再复盘。
- 给出：版本（`git describe --tags` 或安装包版本）、复现步骤、期望与实际行为、影响面评估。
