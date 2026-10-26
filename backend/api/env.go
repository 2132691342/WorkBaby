// 环境引导 handler：闸门快照、PI 的安装与更新、命令执行环境的自愈与终端入口。
// 安装过程以 chat:setup 事件流式回传：npm 可能跑几十秒，没有进度界面会被当成卡死。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// EnvStatus 返回环境闸门快照。
func (h *Handler) EnvStatus(c *gin.Context) {
	ok(c, h.buildPiEnv())
}

// EnvRecheck 重新探测环境并落盘自愈结果：用户刚装了 Node / Git、刚改过 PATH 时点这个。
func (h *Handler) EnvRecheck(c *gin.Context) {
	if h.PiEnv != nil {
		// 工具链解析是进程内缓存：用户刚装完运行时，不先作废就还是走旧结论。
		h.PiEnv.RefreshProbes()
		if exe, _ := h.PiEnv.PiExe(); exe != "" {
			if _, err := h.PiEnv.EnsureShell(); err != nil {
				pkg.Warnf("env: 重新探测命令执行环境失败: %v", err)
			}
		}
	}
	ok(c, h.buildPiEnv())
}

// EnvFixShell 重新探测命令执行环境并写回 PI 的 settings.json。
func (h *Handler) EnvFixShell(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	if _, err := h.Svc.Engine.FixShell(); err != nil {
		fail(c, err)
		return
	}
	ok(c, h.buildPiEnv())
}

// EnvEnableTerminal 生成终端里的 pi 入口并挂到用户 PATH。
func (h *Handler) EnvEnableTerminal(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	if _, err := h.Svc.Engine.EnableTerminal(); err != nil {
		fail(c, err)
		return
	}
	ok(c, h.buildPiEnv())
}

// EnvTerminalLevel 切换终端入口的 PATH 级别：写系统级时后端会拉起 UAC 授权。
func (h *Handler) EnvTerminalLevel(c *gin.Context) {
	var req domain.TerminalLevelREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	st, err := h.Svc.Engine.SwitchTerminalLevel(req.Level)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.TerminalLevelRESP{
		Level: st.Level, OnPath: st.OnPath, Ready: st.Ready, Error: st.Error,
	})
}

// EnvToolchain 返回运行时工具链快照：node / git / pi 各自的来源、版本与 PATH 落地情况。
func (h *Handler) EnvToolchain(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	ok(c, h.Svc.Engine.Toolchain())
}

// EnvPromote 把运行时目录提到系统级 / 降回用户级 / 摘掉：写系统级由后端拉起 UAC。
func (h *Handler) EnvPromote(c *gin.Context) {
	var req domain.PromoteREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	vo, err := h.Svc.Engine.Promote(req.Component, req.Level)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// EnvSwitchNode 换一份 node 重新绑定：后续升级与插件增删改都跟着走这份解释器。
func (h *Handler) EnvSwitchNode(c *gin.Context) {
	var req domain.NodeSwitchREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	vo, err := h.Svc.Engine.SwitchNode(req.Exe)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// EnvEnableBundledGit 解压内置 Git 并接管：机器上一份 git 都没有时的兜底。
func (h *Handler) EnvEnableBundledGit(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	vo, err := h.Svc.Engine.EnableBundledGit()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// buildPiEnv 跑一遍闸门判定。
func (h *Handler) buildPiEnv() *domain.PiEnvVO {
	if h.PiEnv == nil {
		return &domain.PiEnvVO{Blocking: "node", NodeError: "运行环境未装配"}
	}
	modelReady := h.Svc != nil && h.Svc.Providers.HasUsable()
	vo := h.PiEnv.Detect(modelReady)
	out := &domain.PiEnvVO{
		NodeReady: vo.NodeReady, NodePath: vo.NodePath,
		NodeVersion: vo.NodeVersion, NodeSource: vo.NodeSource, NodeError: vo.NodeError,
		NodeBound: vo.NodeBound, NodeLevel: vo.NodeLevel, NodeConflict: vo.NodeConflict,
		GitReady: vo.GitReady, GitPath: vo.GitPath,
		GitVersion: vo.GitVersion, GitSource: vo.GitSource, GitLevel: vo.GitLevel,
		PiReady: vo.PiReady, PiPath: vo.PiPath,
		PiVersion: vo.PiVersion, PiSource: vo.PiSource, PiError: vo.PiError,
		AgentDir:   vo.AgentDir,
		ModelReady: vo.ModelReady, Ready: vo.Ready, Blocking: vo.Blocking,
	}
	shell := h.PiEnv.ShellStatusOf()
	out.ShellKind, out.ShellPath = shell.Kind, shell.Path
	out.ShellUsable, out.ShellError = shell.Usable, shell.Error
	terminal := h.PiEnv.TerminalStatusOf()
	out.TerminalReady, out.TerminalPath = terminal.Ready, terminal.Path
	out.TerminalOnPath, out.TerminalError = terminal.OnPath, terminal.Error
	if h.Svc != nil && h.Svc.Engine != nil {
		out.InstallError = h.Svc.Engine.LastInstallError()
	}
	return out
}

// InstallPi 安装 PI；进度走 SSE（安装是后台动作，接口立刻返回）。
// 失败原因同时落进引擎服务的记录，向导页下一轮轮询就能看到「上一步到底为什么失败」。
func (h *Handler) InstallPi(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	go h.Svc.Engine.Install("", h.setupSink())
	ok(c, true)
}

// emitSetup 推一条安装进度事件。
// 会话 id 留空：安装发生在任何会话之前，事件只是给向导页听。
func (h *Handler) emitSetup(phase string, progress int, message string) {
	if h.Emitter == nil {
		return
	}
	h.Emitter.Emit("", domain.EventChatSetup, domain.SetupData{
		Phase: phase, Progress: progress, Message: message,
	})
}

// emitSetupDone 推一条「这一趟结束」事件：前端靠它把 loading 收回来，
// 不靠定时器猜「应该好了」。
func (h *Handler) emitSetupDone(phase string, ok bool, message string) {
	if h.Emitter == nil {
		return
	}
	h.Emitter.Emit("", domain.EventChatSetup, domain.SetupData{
		Phase: phase, Progress: 100, Message: message, Done: true, OK: ok,
	})
}

// setupSink 把安装输出接到事件流，同时保留最后一行给人看。
func (h *Handler) setupSink() func(string) {
	return func(line string) { h.emitSetup("install", 0, line) }
}
