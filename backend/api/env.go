// 环境引导 handler：闸门快照、PI 的安装与更新。
// 安装过程以 chat:setup 事件流式回传：npm 可能跑几十秒，没有进度界面会被当成卡死。
package api

import (
	"WorkBaby/backend/domain"

	"github.com/gin-gonic/gin"
)

// EnvStatus 返回环境闸门快照。
func (h *Handler) EnvStatus(c *gin.Context) {
	ok(c, h.buildPiEnv())
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
		PythonReady: vo.PythonReady, PythonPath: vo.PythonPath,
		PythonVersion: vo.PythonVersion, PythonSource: vo.PythonSource,
		PiReady: vo.PiReady, PiPath: vo.PiPath,
		PiVersion: vo.PiVersion, PiSource: vo.PiSource, PiError: vo.PiError,
		AgentDir:   vo.AgentDir,
		ModelReady: vo.ModelReady, Ready: vo.Ready, Blocking: vo.Blocking,
	}
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

// setupSink 把安装输出接到事件流，同时保留最后一行给人看。
func (h *Handler) setupSink() func(string) {
	return func(line string) { h.emitSetup("install", 0, line) }
}
