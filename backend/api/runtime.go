// 运行时 handler：内置运行时与 PI 的状态检测、重新检测。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/runtime"
	"github.com/gin-gonic/gin"
)

// RuntimeStatus 报告内置运行时是否就绪，附带失败原因。
func (h *Handler) RuntimeStatus(c *gin.Context) { ok(c, h.runtimeInfo()) }

// RedetectRuntime 重新探测内置运行时：先清探测缓存再跑一遍，
// 首次解压失败（磁盘满、被安全软件拦）修好后，这里点一下就能重试。
func (h *Handler) RedetectRuntime(c *gin.Context) {
	runtime.ResetProbe()
	ok(c, h.runtimeInfo())
}

// runtimeInfo 把运行时探测结果整成界面能直接显示的形状。
// 内置缺失但系统装了可用版本时标注 source=system，而不是报不可用。
func (h *Handler) runtimeInfo() domain.RuntimeInfoVO {
	p := h.Paths
	py := runtime.Status(p)
	if py.Exe != "" {
		py.Source = "bundled"
	} else if sys := runtime.PythonSystemExe(); sys != "" {
		py.Exe, py.Source = sys, "system"
	}
	// Node 与 PI 一并报：少了它们，界面就说不清 PI 为什么起不来。
	nodeSt := runtime.NodeStatusOf(p)
	if nodeSt.Exe == "" {
		if sys := runtime.NodeSystemExe(); sys != "" {
			nodeSt.Exe, nodeSt.Source = sys, "system"
		}
	}
	piExe, piSource, piVersion, piErr := "", "", "", ""
	if h.PiEnv != nil {
		if exe, src := h.PiEnv.PiExe(); exe != "" {
			piExe, piSource = exe, src
			piVersion = h.PiEnv.PiVersion(exe)
		} else {
			piErr = "还没有安装助手引擎（PI）"
		}
	}
	return domain.RuntimeInfoVO{
		NodeExe:     nodeSt.Exe,
		NodeSource:  nodeSt.Source,
		NodeVersion: nodeSt.Version,
		NodeError:   nodeSt.Err,
		PiExe:       piExe,
		PiSource:    piSource,
		PiVersion:   piVersion,
		PiError:     piErr,

		PythonExe:     py.Exe,
		PythonSource:  py.Source,
		PythonVersion: py.Version,
		PythonError:   py.Err,
	}
}

// runtimeReady 报告内置 Python 是否可用。
func runtimeReady(p runtime.Paths) bool { return runtime.PythonExe(p) != "" }
