package pkg

// 业务码是数字字符串：HTTP 状态码已经是 int，信封里再放一个 int 就没法区分
// 「谁失败了」。码值与 docs/ERROR-CODES.md 一一对应，新增码先改文档再写代码。
const (
	// CodeOK 成功；唯一不视为异常的业务码
	CodeOK = "0"

	// 1000-1999 通用 / 文件 / git
	CodeBadRequest   = "1000" // 入参不合法
	CodeFileNotFound = "1001" // 文件或目录不存在
	CodePathOutside  = "1002" // 路径越界：不在允许的根目录内
	CodeFileIO       = "1003" // 文件读写失败
	CodeGitMissing   = "1100" // 内置 git 不可用
	CodeGitFailed    = "1101" // git 命令失败
	CodeGitAuth      = "1102" // git 凭证被拒或无法登录

	// 2000-2999 配置 / 本地存储
	CodeConfigRead  = "2000" // 配置读取失败
	CodeConfigWrite = "2001" // 配置写入失败
	CodeStorage     = "2002" // 本地存储文件操作失败

	// 3000-3999 服务商 / 模型
	CodeProviderBad = "3000" // 服务商配置不合法

	// 5000-5999 会话运行
	CodeSessionBind = "5000" // 引擎未绑定到会话或工作区
	CodeSessionIO   = "5001" // 会话文件读写失败
	CodeEngineBusy  = "5002" // 有任务正在占用引擎
	CodeNotFound    = "5003" // 目标不存在

	// 7000-7999 外部进程
	CodeProvision   = "7000" // 安装 / 解压 / 装载失败
	CodeRuntimeDown = "7001" // 内置运行时未就绪

	// 8000-8999 技能
	CodeSkillBad    = "8000" // 技能路径或内容不合法
	CodeSkillSave   = "8001" // 技能保存失败
	CodeSkillDelete = "8002" // 技能删除失败

	// 9100-9199 MCP
	CodeMcpRead  = "9100" // mcp.json 读取失败
	CodeMcpWrite = "9101" // mcp.json 写入失败

	// 9700-9799 引擎 RPC
	CodeRpcCommand = "9700" // 引擎返回 success=false
	CodeRpcDown    = "9701" // 引擎子进程不在运行
	CodeRpcTimeout = "9702" // 引擎命令超时

	// 兜底
	CodeUnknownRoute = "1404" // 未知路由
	CodeInternal     = "9999" // 未归类的内部错误
)
