// 业务 handler 的公共外壳：统一响应、参数解析与启动引导。编排一律下沉到 service 层。
package api

import (
	"errors"
	"io"
	"strconv"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// bindOptionalBody 解析「可带可不带」的请求体。
// 没 body 不报错（参数还能从 query / path 兜），body 是坏 JSON 则必须当场报错。
func bindOptionalBody(c *gin.Context, out any) error {
	if err := c.ShouldBindJSON(out); err != nil && !errors.Is(err, io.EOF) {
		return pkg.Wrap("1107", "请求格式不正确", err)
	}
	return nil
}

// queryInt 读整数查询参数；缺失或坏值给 0（调用方自己决定默认）。
func queryInt(c *gin.Context, key string) int {
	n, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return 0
	}
	return n
}

// ok 统一成功响应。data 先过一遍空切片归一化：契约说「这是数组」就必须发数组，
// 不能发 null——前端读 null.length 就是一句崩在页面上的报错。
func ok(c *gin.Context, data any) {
	c.JSON(200, domain.Resp{Code: "0", Data: pkg.FillNilSlices(data)})
}

// fail 统一失败响应：code 取自 AppError，前端据此分流。
func fail(c *gin.Context, err error) {
	c.JSON(200, domain.Resp{Code: pkg.CodeOf(err), Message: err.Error()})
}

// Health 存活探测。
func (h *Handler) Health(c *gin.Context) { ok(c, domain.HealthRESP{OK: true}) }

// Bootstrap 启动引导数据：一次给全前端起步要读的东西。
func (h *Handler) Bootstrap(c *gin.Context) {
	settings, err := h.Svc.Settings.All()
	if err != nil {
		fail(c, err)
		return
	}
	provider, model := h.Svc.Providers.EffectiveDefaults()
	ok(c, domain.BootstrapVO{
		Version:         h.Version,
		ContractVersion: domain.ContractVersion,
		DefaultProvider: provider,
		DefaultModel:    model,
		Workspace:       h.workspace(),
		PiEnv:           h.buildPiEnv(),
		Settings:        settings,
	})
}

// Warmup 首屏预热：设置页各面板的清单一次拉齐（只读磁盘与配置，不拉起 PI 进程）。
func (h *Handler) Warmup(c *gin.Context) {
	var (
		skills    []domain.SkillVO
		providers *domain.ProviderListRESP
		tools     *domain.ToolListRESP
		exts      *domain.ExtensionListRESP
		mcp       *domain.MCPListRESP
		wg        sync.WaitGroup
	)
	wg.Add(5)
	go func() { defer wg.Done(); skills = h.Svc.Skills.List() }()
	go func() { defer wg.Done(); providers, _ = h.Svc.Providers.List() }()
	go func() { defer wg.Done(); tools = h.Svc.Tools.Catalog() }()
	go func() { defer wg.Done(); exts, _ = h.Svc.Extensions.List() }()
	go func() {
		defer wg.Done()
		m, err := h.Svc.MCP.List(h.workspace())
		if err == nil {
			mcp = m
		}
	}()
	wg.Wait()

	// 某一路失败时单独补一次：这些都只是读目录与文件，失败多半是并发窗口。
	if providers == nil {
		providers, _ = h.Svc.Providers.List()
	}
	if tools == nil {
		tools = h.Svc.Tools.Catalog()
	}
	if mcp == nil {
		mcp = &domain.MCPListRESP{Servers: []domain.MCPServerVO{}}
	}
	ok(c, domain.WarmupRESP{
		Skills: skills, Providers: providers, Tools: tools, Extensions: exts, MCP: mcp,
	})
}

// workspace 返回当前默认工作目录（配置项优先）。
func (h *Handler) workspace() string {
	if h.Svc != nil {
		if ws := h.Svc.Env.Workspace(); ws != "" {
			return ws
		}
	}
	return h.Cfg.Workspace
}

// 编译期确认 pi 包被引用（session 索引在 service 层使用）。
var _ = pi.ListSessions
