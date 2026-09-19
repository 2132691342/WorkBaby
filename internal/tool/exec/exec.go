// Package exec 提供命令执行工具；走 ExecPolicy 白名单 + 危险命令拦截。
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// ExecTool 执行白名单内命令；参数数组形式，绝不拼 shell。
type ExecTool struct {
	policy    tool.ExecPolicy
	approver  tool.Approver                    // 兜底审批门；仅在脱离 runner 护栏链直调时生效
	pathDirs  func() []string                  // 内置运行时 bin 目录提供者；nil = 不增强 PATH
	whitelist func() []string                  // 可选；非 nil 时每次执行动态覆盖 policy.AllowedBinaries（运行时设置页白名单）
	root      func(ctx context.Context) string // 会话工作区根；nil = cwd 缺省用进程当前目录
	sandbox   func(ctx context.Context) string // 会话工作区 .workbaby 根；用于 cwd 越界校验（防污染用户目录）
}

// New 构造 ExecTool；policy 由装配方（service）注入。
func New(policy tool.ExecPolicy) *ExecTool { return &ExecTool{policy: policy} }

// WithApprover 注入审批门（service 层 ApprovalService 实现 tool.Approver）。
// 仅作为脱离 runner 护栏链直调时的兜底；链内调用由策略门统一裁决。
func (t *ExecTool) WithApprover(a tool.Approver) *ExecTool { t.approver = a; return t }

// ClassifyArgs 实现 tool.RiskClassifier：按本次命令给出审批描述与 per-call 风险。
// 白名单内安全命令返回 (command, "")——策略门据此免审放行。
func (t *ExecTool) ClassifyArgs(args json.RawMessage) (string, string) {
	var req execReq
	if err := json.Unmarshal(args, &req); err != nil {
		return "", ""
	}
	full := req.Command
	if len(req.Args) > 0 {
		full += " " + strings.Join(req.Args, " ")
	}
	if strings.TrimSpace(full) == "" {
		return "", ""
	}
	ok, risk := t.effectivePolicy().Classify(full)
	if ok {
		return full, ""
	}
	if risk == "" {
		return "", "" // 空命令：交 Execute 硬拒绝，不走审批
	}
	return full, risk
}

// WithRootResolver 注入会话工作区根解析器（tool.ResolveRoot 包装 RootResolver）；
// 入参 cwd 缺省且解析出有效根时，命令在该目录执行——与 file 系工具的沙箱根一致。
func (t *ExecTool) WithRootResolver(f func(context.Context) string) *ExecTool { t.root = f; return t }

// WithSandbox 注入会话工作区 .workbaby 根解析器；cwd 越界校验的硬约束：
// LLM 显式传 cwd 时必须落在 workspace 根（含其下的 .workbaby/ 子树）下，否则 4007/4008 拒绝。
// 防止过程脚本落到用户原有目录、污染项目结构。
func (t *ExecTool) WithSandbox(f func(context.Context) string) *ExecTool { t.sandbox = f; return t }

// checkCwd 校验 LLM 传入的 cwd：含 `..` 直接拒绝，且必须落在 workspace 根（含 .workbaby/）内。
// 未绑定工作区（sandbox 为空）放行。
func (t *ExecTool) checkCwd(cwd string, ctx context.Context) *pkg.AppError {
	if strings.Contains(cwd, "..") {
		return pkg.New(4007, "cwd contains '..' path traversal", cwd)
	}
	if t.sandbox == nil {
		return nil
	}
	workspace := ""
	if t.root != nil {
		workspace = strings.TrimSpace(t.root(ctx))
	}
	if workspace == "" {
		return nil
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return pkg.Wrap(4007, "cwd abs resolve failed", err)
	}
	absWS, err := filepath.Abs(workspace)
	if err != nil {
		return nil
	}
	// Windows 路径大小写不敏感：统一 ToLower 比较（更稳，避免 EqualFold 在混合分隔符下漏判）
	lowerCwd := strings.ToLower(filepath.Clean(absCwd))
	lowerWS := strings.ToLower(filepath.Clean(absWS))
	if lowerCwd != lowerWS && !strings.HasPrefix(lowerCwd, lowerWS+string(filepath.Separator)) {
		return pkg.New(4008, "cwd outside workspace sandbox", cwd)
	}
	return nil
}

// WithPathDirs 注入内置运行时 bin 目录提供者；执行时实时读取并前置到子进程 PATH，
// 实现 node/python/pwsh 内置环境隔离（不污染用户环境）。
func (t *ExecTool) WithPathDirs(f func() []string) *ExecTool { t.pathDirs = f; return t }

// WithWhitelist 注入运行时白名单提供者（system_settings 读取），每次执行实时生效。
func (t *ExecTool) WithWhitelist(f func() []string) *ExecTool { t.whitelist = f; return t }

// effectivePolicy 返回本次执行的生效策略（运行时白名单覆盖构造时默认值）。
func (t *ExecTool) effectivePolicy() tool.ExecPolicy {
	p := t.policy
	if t.whitelist != nil {
		p.AllowedBinaries = t.whitelist()
	}
	return p
}

func (t *ExecTool) Name() string              { return "exec" }
func (t *ExecTool) RiskLevel() tool.RiskLevel { return tool.RiskExec }

func (t *ExecTool) Description() string {
	return "执行白名单内的本机命令（参数数组形式，无 shell 拼接）。必须传入完整命令与参数列表。"
}

func (t *ExecTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["command"],
			"properties": {
				"command": {"type": "string", "description": "可执行文件路径或名称，必须命中白名单"},
				"args": {"type": "array", "items": {"type": "string"}, "description": "参数列表（数组形式，禁止 shell 拼接）"},
				"timeout": {"type": "integer", "description": "超时毫秒，缺省用策略默认值"},
				"cwd": {"type": "string", "description": "工作目录，缺省为会话工作区（绑定了外部目录时）"}
			}
		}`),
	}
}

// execReq 入参。
type execReq struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"` // ms；<=0 用策略默认
	Cwd     string   `json:"cwd"`
}

// Meta 声明：命令执行类（前端按 exec 类别呈现，结果按输出上限截断）。
func (t *ExecTool) Meta() tool.ToolMeta {
	return tool.ToolMeta{
		Group: tool.GroupExec, Category: tool.CategoryExec, ActivityDesc: "执行命令",
		MaxResultChars: execMaxOutputBytes, TimeoutSec: 120,
	}
}

// ActivityDescription 时间线文案显示真实命令：用户最需要看到的就是「它要干什么」。
func (t *ExecTool) ActivityDescription(args json.RawMessage) string {
	var req execReq
	if json.Unmarshal(args, &req) != nil || req.Command == "" {
		return ""
	}
	line := req.Command
	if len(req.Args) > 0 {
		line += " " + strings.Join(req.Args, " ")
	}
	if r := []rune(line); len(r) > 60 {
		line = string(r[:60]) + "…"
	}
	return "正在执行 " + line
}

// 回填前的输出读取硬上限（先限流再截断，避免 `find /`、`cat 大文件` 把全量输出
// 读进内存造成 OOM）。分双端保留：只留头部会丢掉错误栈与最终统计（最常要看的），
// 只留尾部会丢掉命令回显与首屏上下文。
const (
	execHeadBytes      = 120 << 10 // 头部保留
	execTailBytes      = 80 << 10  // 尾部环形保留
	execMaxOutputBytes = execHeadBytes + execTailBytes
)

// cappedWriter 合并 stdout/stderr 的双端限流写入器：头 120KB + 尾 80KB，
// 中间丢弃并计数（丢掉的字节仍计入返回值，保证子进程不因管道写满而阻塞）。
type cappedWriter struct {
	head    bytes.Buffer
	tail    []byte // 环形缓冲；未写满时用 tailLen 标记有效长度
	tailPos int
	tailLen int
	total   int64
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.total += int64(n)

	// 头部未满：优先填头
	if room := execHeadBytes - w.head.Len(); room > 0 {
		if len(p) <= room {
			w.head.Write(p)
			return n, nil
		}
		w.head.Write(p[:room])
		p = p[room:]
	}
	// 头部已满：进环形尾
	if w.tail == nil {
		w.tail = make([]byte, execTailBytes)
	}
	if len(p) >= execTailBytes {
		copy(w.tail, p[len(p)-execTailBytes:])
		w.tailPos, w.tailLen = 0, execTailBytes
		return n, nil
	}
	first := copy(w.tail[w.tailPos:], p)
	copy(w.tail, p[first:])
	w.tailPos = (w.tailPos + len(p)) % execTailBytes
	if w.tailLen < execTailBytes {
		w.tailLen += len(p)
		if w.tailLen > execTailBytes {
			w.tailLen = execTailBytes
		}
	}
	return n, nil
}

// dropped 被丢弃的字节数（双端之间未保留的部分）。
func (w *cappedWriter) dropped() int64 {
	return w.total - int64(w.head.Len()) - int64(w.tailLen)
}

// Output 拼装最终回填文本：头 + 丢弃提示 + 尾。
func (w *cappedWriter) Output() string {
	var sb strings.Builder
	sb.WriteString(w.head.String())
	if d := w.dropped(); d > 0 {
		fmt.Fprintf(&sb, "\n... (output truncated, %d bytes dropped) ...\n", d)
	}
	if w.tailLen > 0 {
		sb.Write(w.tailBytes())
	}
	return sb.String()
}

// tailBytes 环形缓冲按写入顺序还原（最旧 → 最新）。
func (w *cappedWriter) tailBytes() []byte {
	if w.tailLen == 0 {
		return nil
	}
	if w.tailLen < execTailBytes {
		return w.tail[:w.tailLen]
	}
	out := make([]byte, execTailBytes)
	n := copy(out, w.tail[w.tailPos:])
	copy(out[n:], w.tail[:w.tailPos])
	return out
}

// Execute 执行命令并返回合并输出（限流读取，上限 execMaxOutputBytes）。
func (t *ExecTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req execReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "exec args parse failed", err)}
	}
	// 白名单按 basename 匹配 command；危险模式检查完整命令串（command + args）
	full := req.Command
	if len(req.Args) > 0 {
		full += " " + strings.Join(req.Args, " ")
	}
	// 安全分类：白名单内直接放行；白名单外/危险命令按护栏链裁决。
	// 统一护栏链（runner 策略门 + 单次审批）已裁决时不再重复询问——单层闸门；
	// 脱链直调（测试/裸用）保留审批兜底，fail-closed 语义不变。
	policy := t.effectivePolicy()
	if ok, risk := policy.Classify(full); !ok {
		if risk == "" {
			return tool.ToolResult{Err: pkg.New(4001, tool.ErrBinaryDenied.Message, "<empty>")}
		}
		switch {
		case tool.GuardChainActive(ctx):
			// runner 护栏链已放行（含审批通过），不再二次询问
		case t.approver != nil:
			if !t.approver.Approve(ctx, full, risk) {
				return tool.ToolResult{Err: pkg.New(4003, tool.ErrApprovalNeeded.Message+", user denied or timed out", full)}
			}
		case risk == tool.RiskApprovalIrrev:
			return tool.ToolResult{Err: pkg.New(4002, tool.ErrPatternDenied.Message, full)}
		default:
			return tool.ToolResult{Err: pkg.New(4001, tool.ErrBinaryDenied.Message, req.Command)}
		}
	}

	execCtx := ctx
	timeout := policy.Timeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Millisecond
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 平台侧解析：Windows 上 .cmd 外壳与 cmd 内建命令需包 cmd /c 才能 CreateProcess。
	// 关键：LookPath 用父进程 PATH，而父进程 PATH 不含内置运行时——
	// 必须先临时把内置 bin 目录前置到 PATH 再查找，否则内置 node/python/pwsh
	// 在用户系统未装时被 LookPath 漏掉，回落到 cmd /c 后内置环境彻底失联。
	var dirs []string
	if t.pathDirs != nil {
		dirs = t.pathDirs()
	}
	exe, prefix := resolveWithBuiltin(req.Command, dirs)
	cmd := exec.CommandContext(execCtx, exe, append(prefix, req.Args...)...)
	switch {
	case req.Cwd != "":
		// cwd 越界校验：防 LLM 把过程脚本写到用户目录外（如 `C:\Users\xxx`），
		// 也防 `..` 路径穿越（如 `D:\foo\..\bar` 落到 D:\bar）。
		if err := t.checkCwd(req.Cwd, ctx); err != nil {
			return tool.ToolResult{Err: err}
		}
		cmd.Dir = req.Cwd
	case t.root != nil:
		// 工作区联动：解析出的根必须真实存在才生效，否则维持进程当前目录
		//（默认会话隔离目录可能尚未创建，缺目录不应让命令直接失败）。
		if root := strings.TrimSpace(t.root(ctx)); root != "" {
			if info, serr := os.Stat(root); serr == nil && info.IsDir() {
				cmd.Dir = root
			}
		}
	}
	if t.pathDirs != nil {
		if dirs := t.pathDirs(); len(dirs) > 0 {
			cmd.Env = envWithPath(dirs) // 子进程 PATH 前置内置运行时目录
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // Windows 不弹控制台

	// stdout/stderr 合并限流读取（CombinedOutput 无上限，全量入内存有 OOM 风险）
	out := &cappedWriter{}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	meta := map[string]string{"exitCode": strconv.Itoa(exitCode)}
	if d := out.dropped(); d > 0 {
		meta["truncatedBytes"] = strconv.FormatInt(d, 10)
	}
	output := out.Output()
	if err != nil {
		if execCtx.Err() != nil {
			return tool.ToolResult{Content: output, Meta: meta, Err: pkg.Wrap(4006, "exec timeout or cancelled", err)}
		}
		return tool.ToolResult{Content: output, Meta: meta, Err: pkg.Wrap(4006, "exec failed", err)}
	}
	return tool.ToolResult{Content: output, Meta: meta}
}

// envWithPath 返回在现有环境基础上把 dirs 前置到 PATH 的环境切片。
//
// Windows 的环境变量名大小写不敏感，os.Environ() 可能返回 "Path=" 而非 "PATH="：
// 按字面前缀匹配会漏掉，导致原 PATH 被丢弃、子进程只剩内置运行时目录。
// 这里用 os.Getenv（Windows 上大小写不敏感）取值，再按不区分大小写的键名整条替换。
func envWithPath(dirs []string) []string {
	sep := string(os.PathListSeparator)
	extra := strings.Join(dirs, sep)
	pathVal := os.Getenv("PATH")
	if pathVal == "" {
		pathVal = extra
	} else {
		pathVal = extra + sep + pathVal
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if i := strings.Index(kv, "="); i > 0 && strings.EqualFold(kv[:i], "PATH") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "PATH="+pathVal)
}

// lookPathIn 只在内置运行时目录中解析命令，不触碰进程全局 PATH。
//
// exec.LookPath 收到含路径分隔符的入参时会直接检查该文件（Windows 下仍按 PATHEXT
// 尝试扩展名），因此「内置目录优先」无需改写 PATH 即可实现。
func lookPathIn(name string, dirs []string) (string, error) {
	var lastErr error
	for _, d := range dirs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		lp, err := exec.LookPath(filepath.Join(d, name))
		if err == nil {
			return lp, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return "", lastErr
}

// resolveWithBuiltin 在内置运行时 bin 目录优先的前提下解析命令名，返回 (abs path, prefix args)。
// 内置目录未命中时回落进程 PATH；解析过程不改动进程全局 PATH（并发 exec 下是数据竞争）。
func resolveWithBuiltin(name string, dirs []string) (string, []string) {
	if lp, err := lookPathIn(name, dirs); err == nil {
		return wrapShell(lp)
	}
	return resolveCommand(name)
}
