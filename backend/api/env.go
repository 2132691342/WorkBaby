// 环境引导 handler：闸门快照、PI 的安装与更新。
// 安装过程以 chat:setup 事件流式回传：npm 可能跑几十秒，没有进度界面会被当成卡死。
package api

import (
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// installMu 保证同一时刻只有一次安装：两次 npm install 并发会把 node_modules 写坏。
var installMu sync.Mutex

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
	return &domain.PiEnvVO{
		NodeReady: vo.NodeReady, NodePath: vo.NodePath,
		NodeVersion: vo.NodeVersion, NodeSource: vo.NodeSource, NodeError: vo.NodeError,
		PiReady: vo.PiReady, PiPath: vo.PiPath,
		PiVersion: vo.PiVersion, PiSource: vo.PiSource, PiError: vo.PiError,
		ModelReady: vo.ModelReady, Ready: vo.Ready, Blocking: vo.Blocking,
	}
}

// InstallPi 安装或更新 PI。
// 系统能力绑定之外它只是触发一次 npm，进度走 SSE（install 是后台动作，接口立刻返回）。
func (h *Handler) InstallPi(c *gin.Context) {
	if h.PiEnv == nil {
		fail(c, pkg.New("9704", "运行环境未装配", ""))
		return
	}
	if !installMu.TryLock() {
		fail(c, pkg.New("9710", "正在安装，等这一趟结束", ""))
		return
	}
	go func() {
		defer installMu.Unlock()
		h.emitSetup("install", 0, "正在安装助手引擎…")
		err := h.PiEnv.InstallPi(func(line string) {
			// npm 的每行输出都推给前端：看不到动就是在卡死。
			h.emitSetup("install", 0, line)
		})
		if err != nil {
			h.emitSetup("install", 0, "安装失败："+err.Error())
			pkg.Errorf("env: 安装 PI 失败: %v", err)
			return
		}
		h.emitSetup("install", 100, "助手引擎已就绪")
	}()
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
