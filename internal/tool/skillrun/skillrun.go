// Package skillrun 把 Skill 内置脚本暴露为 Agent 可调用工具。
// 脚本来自 skills 表 ScriptsJSON，解释器按 language 固定映射，参数数组直传不经 shell；
// 脚本落临时文件用完即删，执行前经审批门，内置运行时 bin 目录前置 PATH。
package skillrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/tool"
)

const (
	toolName     = "run_skill_script"
	scriptMaxLen = 256 * 1024 // 单脚本上限 256KB
	runTimeout   = 2 * time.Minute
)

// ScriptResolver 按 (skill, script) 解析脚本内容与语言；service 层注入（skills 表唯一真相源）。
type ScriptResolver func(skillName, scriptName string) (code, language string, err error)

// skillScratchDir 过程脚本落点：工作区沙箱 <workspace>/.workbaby/tmp（按需创建），
// 未绑定工作区或沙箱不可写时回落系统临时目录。
func skillScratchDir(root string) (string, error) {
	if base := runtime.SandboxFile(root, runtime.SubTmp); base != "" {
		if err := os.MkdirAll(base, 0o755); err == nil {
			if d, err := os.MkdirTemp(base, "wb-skill-*"); err == nil {
				return d, nil
			}
		}
	}
	return os.MkdirTemp("", "wb-skill-*")
}

// interpreter 语言 → (解释器, 脚本扩展名)。固定映射，绝不拼 shell。
func interpreter(lang string) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "javascript", "js", "node":
		return "node", ".js", true
	case "python", "python3", "py":
		return "python", ".py", true
	case "powershell", "pwsh", "ps1":
		return "powershell", ".ps1", true
	}
	return "", "", false
}

// SkillRunTool run_skill_script 工具。
type SkillRunTool struct {
	resolve  ScriptResolver
	defRoot  string            // 默认工作区（未绑定会话目录时脚本 cwd）
	rootRes  tool.RootResolver // 会话工作区解析器；nil = 恒用 defRoot
	approver tool.Approver     // 兜底审批门；仅在脱离 runner 护栏链直调时生效
	pathDirs func() []string
}

// New 构造；workspace 为脚本执行 cwd。
func New(resolve ScriptResolver, workspace string) *SkillRunTool {
	return &SkillRunTool{resolve: resolve, defRoot: workspace}
}

// WithRootResolver 注入会话工作区解析器：脚本 cwd 跟随会话绑定的目录。
func (t *SkillRunTool) WithRootResolver(r tool.RootResolver) *SkillRunTool {
	t.rootRes = r
	return t
}

// rootOf 解析本次执行的脚本 cwd（会话绑定目录优先，回落默认根）。
func (t *SkillRunTool) rootOf(ctx context.Context) string {
	return tool.ResolveRoot(t.rootRes, t.defRoot)(ctx)
}

// WithApprover 注入审批门（ApprovalService；同一审批流）。
// 仅作为脱离 runner 护栏链直调时的兜底；链内调用由策略门统一裁决。
func (t *SkillRunTool) WithApprover(a tool.Approver) *SkillRunTool { t.approver = a; return t }

// ClassifyArgs 实现 tool.RiskClassifier：执行代码一律 needs_approval（每次确认）。
func (t *SkillRunTool) ClassifyArgs(args json.RawMessage) (string, string) {
	var req struct {
		Skill  string   `json:"skill"`
		Script string   `json:"script"`
		Args   []string `json:"args"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "", ""
	}
	desc := fmt.Sprintf("%s(%s/%s %s)", toolName, req.Skill, req.Script, strings.Join(req.Args, " "))
	return desc, tool.RiskApprovalNeeds
}

// WithPathDirs 注入内置运行时 bin 目录（node/python 隔离环境）。
func (t *SkillRunTool) WithPathDirs(f func() []string) *SkillRunTool { t.pathDirs = f; return t }

func (t *SkillRunTool) Name() string        { return toolName }
func (t *SkillRunTool) Description() string { return t.schema().Description }

func (t *SkillRunTool) RiskLevel() tool.RiskLevel { return tool.RiskExec }

func (t *SkillRunTool) Schema() tool.ToolSchema { return t.schema() }

func (t *SkillRunTool) schema() tool.ToolSchema {
	params, _ := json.Marshal(map[string]any{
		"type":     "object",
		"required": []string{"skill", "script"},
		"properties": map[string]any{
			"skill":  map[string]any{"type": "string", "description": "Skill 名称"},
			"script": map[string]any{"type": "string", "description": "脚本名（Skill scripts 中的 name）"},
			"args":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "可选：传给脚本的参数"},
		},
	})
	return tool.ToolSchema{
		Name:        toolName,
		Description: "运行 Skill 内置脚本。参数：skill（技能名）、script（脚本名）、args（可选参数数组）。",
		Parameters:  params,
	}
}

// safeScriptName 脚本名文件安全化：剥路径分量与分隔符（防写入越界）。
func safeScriptName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, base)
}

// Execute 解析脚本 → 审批 → 临时文件 → 解释器直跑 → CombinedOutput。
func (t *SkillRunTool) Execute(ctx context.Context, raw json.RawMessage) tool.ToolResult {
	var req struct {
		Skill  string   `json:"skill"`
		Script string   `json:"script"`
		Args   []string `json:"args"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return tool.ToolResult{Err: pkg.New(4004, "invalid args: "+err.Error(), "")}
	}
	if req.Skill == "" || req.Script == "" {
		return tool.ToolResult{Err: pkg.New(4004, "skill and script are required", "")}
	}
	code, lang, err := t.resolve(req.Skill, req.Script)
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	if len(code) > scriptMaxLen {
		return tool.ToolResult{Err: pkg.New(9105, "script too large (>256KB)", req.Script)}
	}
	inter, ext, ok := interpreter(lang)
	if !ok {
		return tool.ToolResult{Err: pkg.New(9105, "unsupported script language: "+lang, req.Script)}
	}

	// 审批：执行代码一律 needs_approval。统一护栏链（runner 策略门）已裁决时不再重复询问；
	// 脱链直调（测试/裸用）保留审批兜底，fail-closed 语义不变。
	desc := fmt.Sprintf("%s(%s/%s %s)", toolName, req.Skill, req.Script, strings.Join(req.Args, " "))
	if !tool.GuardChainActive(ctx) {
		if t.approver == nil {
			return tool.ToolResult{Err: pkg.New(4003, "approval service not configured", toolName)}
		}
		if !t.approver.Approve(ctx, desc, tool.RiskApprovalNeeds) {
			return tool.ToolResult{Err: pkg.New(4003, tool.ErrApprovalNeeded.Message+", user denied or timed out", desc)}
		}
	}

	// 脚本落临时文件（用完即删）：优先落在工作区沙箱 .workbaby/tmp，
	// 让「本次执行用了什么脚本」留在工作区内可追溯；未绑定工作区时回落系统临时目录。
	dir, err := skillScratchDir(t.rootOf(ctx))
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(1002, "mktemp failed", err)}
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, safeScriptName(req.Script)+ext)
	if err := os.WriteFile(path, []byte(code), 0o600); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(1002, "write script failed", err)}
	}

	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	// 内置运行时优先解析解释器：LookPath 走父进程 PATH，必须临时把内置 bin 目录
	// 前置，否则用户系统未装 node/python 时回落到 cmd /c、内置环境彻底失联。
	var dirs []string
	if t.pathDirs != nil {
		dirs = t.pathDirs()
	}
	if len(dirs) > 0 {
		origPath := os.Getenv("PATH")
		os.Setenv("PATH", strings.Join(dirs, string(os.PathListSeparator))+string(os.PathListSeparator)+origPath)
		defer os.Setenv("PATH", origPath)
	}

	cmd := exec.CommandContext(runCtx, inter, append([]string{path}, req.Args...)...)
	cmd.Dir = t.rootOf(ctx)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if len(dirs) > 0 {
		// 子进程 PATH 也前置内置运行时，确保脚本内部调起 python/node 同样命中内置版本
		sep := string(os.PathListSeparator)
		extra := strings.Join(dirs, sep)
		pathVal := os.Getenv("PATH")
		if pathVal != "" {
			pathVal = extra + sep + pathVal
		} else {
			pathVal = extra
		}
		env := make([]string, 0, len(os.Environ())+1)
		for _, kv := range os.Environ() {
			if i := strings.Index(kv, "="); i > 0 && strings.EqualFold(kv[:i], "PATH") {
				continue
			}
			env = append(env, kv)
		}
		cmd.Env = append(env, "PATH="+pathVal)
	}

	out, runErr := cmd.CombinedOutput()
	meta := map[string]string{}
	if ee, ok := runErr.(*exec.ExitError); ok {
		meta["exit_code"] = fmt.Sprint(ee.ExitCode())
	}
	if runErr != nil && len(out) == 0 {
		if runCtx.Err() == context.DeadlineExceeded {
			return tool.ToolResult{Err: pkg.New(9105, "script run timeout", req.Script), Meta: meta}
		}
		return tool.ToolResult{Err: pkg.New(9105, "script run failed: "+runErr.Error(), req.Script), Meta: meta}
	}
	return tool.ToolResult{Content: string(out), Meta: meta}
}
