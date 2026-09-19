package tool

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"WorkBaby/internal/pkg"
)

// ExecPolicy 命令执行策略：白名单 basename 匹配 + 危险命令正则拦截。
//
// 配置来源：构造时注入默认值；运行时可改。
type ExecPolicy struct {
	AllowedBinaries []string      // 白名单（basename 精确匹配）；空 = 全部拒绝
	DeniedPatterns  []string      // 危险命令正则（匹配整条 command + args）
	ApprovalLevel   RiskLevel     // 该等级及以上需审批
	Timeout         time.Duration // 默认单次执行超时

	denied []*regexp.Regexp // DeniedPatterns 的编译产物（Compile 填充；零值回退即时编译）
}

// DefaultExecPolicy 开箱可用默认：内置 Windows 常用工具链白名单（设置页 exec.whitelist 可整体覆盖），
// 破坏性命令正则拦截；审批门槛 = exec 级。
//
// 白名单非空是「Agent 能干活」的前提——空白名单会让每条命令都弹审批，
// 长任务在人工响应前就被墙钟预算砍断。安全由「白名单 basename + 破坏性正则 + 审批」三重叠加保证。
func DefaultExecPolicy() ExecPolicy {
	return ExecPolicy{
		AllowedBinaries: []string{
			// Windows shell：内建命令（dir/type/copy…）与管道/重定向必须经它
			"cmd", "powershell", "pwsh",
			// 版本管理与构建工具链
			"git", "go", "node", "npm", "npx", "pnpm", "yarn",
			"python", "python3", "py", "uv", "uvx",
			"dotnet", "java", "javac", "mvn", "gradle", "make",
			// 网络与归档
			"curl", "wget", "tar", "unzip",
			// 文件与文本检索（cmd 内建，无独立 exe，执行时由 cmd /c 兜底）
			"dir", "type", "echo", "copy", "move", "mkdir", "rmdir", "find", "findstr", "where", "set",
		},
		DeniedPatterns: []string{
			`(?i)\brm\s+(-rf|-fr|-r)\b`,         // 递归强删
			`(?i)del\s+/f\s+/s`,                 // 批量强制删
			`(?i)del\s+/s`,                      // 静默删目录树
			`(?i)rd\s+/s\s+/q`,                  // 静默删目录树
			`(?i)rmdir\s+/s`,                    // 删目录树
			`(?i)format\s+[a-z]:`,               // 格式化盘符
			`(?i)mkfs`,                          // 格式化
			`(?i)diskpart`,                      // 磁盘分区
			`(?i)cleanmgr`,                      // 磁盘清理（交互破坏）
			`(?i)cipher\s+/w`,                   // 空闲空间擦除
			`(?i)reg\s+delete`,                  // 注册表删除
			`(?i)shutdown\s*$|(?i)shutdown\s+-`, // 关机
			`(?i)taskkill\s+/f`,                 // 强杀进程
		},
		ApprovalLevel: RiskExec,
		Timeout:       5 * time.Minute,
	}.Compile()
}

// Compile 预编译危险命令正则并返回带编译产物的副本。
//
// Classify 对每条命令都要匹配全部拒绝模式，逐次 regexp.MustCompile 属热路径浪费；
// 装配方在构造或策略变更后调用一次即可。未编译时 Classify 回退即时编译，语义不变。
func (p ExecPolicy) Compile() ExecPolicy {
	if len(p.DeniedPatterns) == 0 {
		p.denied = nil
		return p
	}
	p.denied = make([]*regexp.Regexp, 0, len(p.DeniedPatterns))
	for _, pat := range p.DeniedPatterns {
		p.denied = append(p.denied, regexp.MustCompile(pat))
	}
	return p
}

// deniedRes 生效的拒绝正则：优先用预编译产物；模式被运行时整体替换（长度不匹配）时即时编译兜底。
func (p ExecPolicy) deniedRes() []*regexp.Regexp {
	if len(p.denied) == len(p.DeniedPatterns) {
		return p.denied
	}
	rs := make([]*regexp.Regexp, 0, len(p.DeniedPatterns))
	for _, pat := range p.DeniedPatterns {
		rs = append(rs, regexp.MustCompile(pat))
	}
	return rs
}

// Allow 校验命令是否允许执行：bin 必须在白名单，整条命令不得命中拒绝模式。
func (p ExecPolicy) Allow(command string) error {
	if ok, risk := p.Classify(command); ok {
		return nil
	} else if risk == RiskApprovalIrrev {
		return pkg.New(4002, ErrPatternDenied.Message, command)
	}
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return pkg.New(4001, ErrBinaryDenied.Message, "<empty>")
	}
	return pkg.New(4001, ErrBinaryDenied.Message, filepath.Base(fields[0]))
}

// Classify 对整条命令（command + args 拼接后）做安全分类：
//   - ok=true                          → 白名单内且未命中危险正则，直接放行
//   - ok=false, risk=RiskApprovalIrrev → 命中危险正则（不可逆），走审批（每次确认）
//   - ok=false, risk=RiskApprovalNeeds → 白名单外（可恢复），走审批
//   - ok=false, risk=""                → 直接拒绝（空命令），不进入审批
func (p ExecPolicy) Classify(command string) (ok bool, risk string) {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false, ""
	}
	whitelisted := slicesContains(p.AllowedBinaries, filepath.Base(fields[0]))
	for _, re := range p.deniedRes() {
		if re.MatchString(command) {
			return false, RiskApprovalIrrev
		}
	}
	if whitelisted {
		return true, ""
	}
	return false, RiskApprovalNeeds
}

func slicesContains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
