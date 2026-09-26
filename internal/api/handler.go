// Package api 组合根 + 系统能力绑定层（薄；一个文件一个功能域）：装配全部 service 与工具，
// 业务 API 由 server 层经 gin HTTP 暴露，本层 Wails 绑定仅系统能力；唯一直接 import wails runtime 的层。
package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/bootstrap"
	"WorkBaby/internal/capability"
	"WorkBaby/internal/config"
	"WorkBaby/internal/agent"
	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm/registry"
	"WorkBaby/internal/mcp"
	"WorkBaby/internal/memory"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/rag"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/service"
	"WorkBaby/internal/skill"
	"WorkBaby/internal/tool"
	archivetool "WorkBaby/internal/tool/archive"
	delegatetool "WorkBaby/internal/tool/delegate"
	doctool "WorkBaby/internal/tool/doc"
	exectool "WorkBaby/internal/tool/exec"
	filetool "WorkBaby/internal/tool/file"
	functools "WorkBaby/internal/tool/functools"
	httptool "WorkBaby/internal/tool/http"
	"WorkBaby/internal/tool/planmode"
	requestinput "WorkBaby/internal/tool/requestinput"
	skillrun "WorkBaby/internal/tool/skillrun"
	todotool "WorkBaby/internal/tool/todo"
	webfetchtool "WorkBaby/internal/tool/webfetch"
	websearchtool "WorkBaby/internal/tool/websearch"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Handler 聚合所有 service，对外导出方法自动成为 Wails 绑定。
// 本包允许唯一直接 import wails runtime，且仅用于 runtime.EventsEmit；
// service / repo / domain 严禁导入 wails。
type Handler struct {
	ctx        context.Context
	paths      *runtime.Paths
	runtimeMgr *runtime.Manager
	cfg        *config.Config
	bus        *event.Bus
	eventLog   *event.RunEventLog      // run 事件日志（序号 + 断线重放缓冲），与 SSE hub 共享
	execs      *agent.ExecutionRegistry // 执行平面：全入口统一 run 登记
	app        *bootstrap.App          // 组合根：repo 装配唯一入口（api 层不 import repo）
	cipher     *pkg.Cipher
	reg        *registry.Registry
	metaSvc    *MetaService
	provSvc    *service.ProviderService
	chatSvc    *service.ChatService
	todoStore  *service.SessionTodoStore // 会话计划存储（todo 工具 + 前端共享）

	setSvc       *service.SettingsService
	toolSvc      *service.ToolService
	dashSvc      *service.DashboardService
	docsSvc      *service.DocsService
	approvalSvc  *service.ApprovalService
	memSvc       *memory.Service
	memProxy     *service.MemoryService
	skillSvc     *service.SkillService
	agentSvc     *service.AgentProfileService
	commandSvc   *service.UserCommandService
	mcpSvc       *service.McpService
	knowledgeSvc *service.KnowledgeService
	taskSvc      *service.ChatTaskService // 后台任务域（提交/列表/取消 + task:* 事件）
	folderSvc    *service.FolderService
	fileSvc      *service.FileService
	workspaceSvc *service.WorkspaceService
	changeSvc    *service.FileChangeService // 文件变更追踪（快照 + diff + 回滚）
	artifactSvc  *service.ArtifactService   // 会话产出物登记
	trustSvc     *service.TrustService      // 工作目录信任
	hookSvc      *service.UserHookService   // 用户钩子（设置页 CRUD + 运行时执行器）
	// Files 服务本地受管文件（main.go AssetServer 转发 /files/**）。
	// 独立类型而非 Handler 方法：避免 net/http 类型泄漏进 Wails 绑定（见 fileserver.go）。
	Files *FileServer
}

// NewHandler 构造壳。
func NewHandler() *Handler {
	return &Handler{
		bus:      event.New(),
		eventLog: event.NewRunEventLog(0, 0),
		execs:    agent.NewExecutionRegistry(0),
	}
}

// Events 暴露应用内事件总线（双主机：server/sse 订阅桥接）。
func (h *Handler) Events() *event.Bus { return h.bus }

// EventLog 暴露 run 事件日志（server/sse 用它做断线重放）。
func (h *Handler) EventLog() *event.RunEventLog { return h.eventLog }

// ExecutionRegistry 暴露执行平面注册表（供 chat 等入口登记统一 run 拓扑）。
func (h *Handler) ExecutionRegistry() *agent.ExecutionRegistry { return h.execs }

// isRegularFile 判断路径是常规文件（非目录）。
func isRegularFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// EmitReady 在 gin server 启动后发射 app:ready（携带 serverPort 供前端建立 HTTP 连接）。
func (h *Handler) EmitReady(serverPort int) {
	if h.ctx == nil {
		return
	}
	wruntime.EventsEmit(h.ctx, "app:ready", map[string]any{
		"home":        h.paths.Home,
		"version":     h.cfg.App.Version,
		"phase":       "dual-host",
		"server_port": serverPort,
	})
}

// Startup 按序装配：路径 → 日志 → 配置 → 主密钥 → DB → 迁移 → repo/service → LLM registry → 事件桥接 → app:ready。
func (h *Handler) Startup(ctx context.Context) error {
	paths, err := runtime.Resolve()
	if err != nil {
		return err
	}
	h.paths = paths

	if err := pkg.Init(paths.Log); err != nil {
		return err
	}

	// 内置运行时（python）：后台首跑解压，不阻塞启动；失败仅告警
	rt := runtime.NewManager(paths.Home)
	// 事件 JSONL 无头导出：每 run 一文件，供回放/测试/自动化消费
	h.eventLog.WithFileSink(filepath.Join(paths.Home, "runs"))
	h.runtimeMgr = rt
	// 同步等待内置运行时解压：MCP.Sync 随后调用，其子进程 PATH 注入依赖 rt.BinDirs()；
	// 异步解压时 uvx 类 server 启动期拿不到 python，只能退回系统 PATH。
	// Ensure 失败仅告警（不阻断启动，但日志可见）。
	if err := rt.Ensure(); err != nil {
		pkg.L.Warn("runtime ensure failed", "err", err)
	}

	cfg, err := config.Init(paths.Cfg)
	if err != nil {
		return err
	}
	if err := config.EnsureUserConfig(paths.Home); err != nil {
		return err
	}
	// config.Init 已经确保 masterKeyB64 不空
	cipher, err := pkg.NewCipherFromB64(cfg.Security.MasterKeyB64)
	if err != nil {
		return err
	}
	h.cipher = cipher
	h.cfg = cfg
	if cfg.Database.Path == "" {
		cfg.Database.Path = paths.DB
	}

	gdb, err := db.Open(cfg.Database.Path, cfg.Database.BusyTimeoutMs)
	if err != nil {
		return err
	}
	if err := db.Migrate(gdb); err != nil {
		return err
	}
	if err := db.CreateFTS5(gdb); err != nil {
		return err
	}

	h.app = bootstrap.New(gdb)
	h.metaSvc = NewMetaService(cfg)
	h.provSvc = service.NewProviderService(h.app.ProvRepo, h.cipher)
	h.setSvc = service.NewSettingsService(h.app.SetRepo, h.cipher)

	// 配置文件为源：model.json 存在则同步进 ai_providers 表
	// （文件缺失/解析失败仅告警，保留 DB 存量）
	if modelPath := service.ModelRawPath(paths.Home); isRegularFile(modelPath) {
		if rows, perr := service.ProvidersFromFile(modelPath); perr != nil {
			pkg.L.Warn("sync model.json failed", "err", perr)
		} else if serr := h.provSvc.SyncFromList(ctx, rows); serr != nil {
			pkg.L.Warn("sync model.json to db failed", "err", serr)
		}
	}

	// LLM Registry：解密已落盘 apiKey 后构建；失败单条降级（unready）
	h.reg = registry.New()
	if pwds, perr := h.app.ProvRepo.List(ctx); perr == nil {
		_ = h.reg.Build(pwds, func(encrypted string) (string, error) {
			return h.cipher.Decrypt(encrypted)
		})
		// 启动后把 enabled 但未就绪的 provider 原因打到日志，
		// 便于排查「每次重启模型都处于熔断/不可用」这类问题（多数是 apiKey 解密失败）。
		for _, it := range h.reg.Status(pwds) {
			if !it.Ready {
				pkg.L.Warn("llm registry provider not ready at startup",
					"provider", it.Name, "reason", it.Reason)
			}
		}
	}

	// 会话路径解析：工作区根与过程数据目录的唯一数据源（工具沙箱 / 快照 / 文件面板 / chat 共用）。
	sctx := service.NewSessionContext(paths.Home, h.app.SessRepo)
	// 事件出口：chat / task / approval / 变更 / 工件 共用同一实例，seq 落在同一条序列上。
	emitter := service.NewEmitter(h.bus, h.eventLog)

	// 工具系统：工作区根 + 内置工具注册（exec/file/webfetch/http/websearch）
	workspace := filepath.Join(paths.Home, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return pkg.Wrap(2001, "mkdir workspace failed", err)
	}
	// 会话工作区解析：会话绑定了外部目录 → 工具沙箱/面板/exec cwd 全部跟随；
	// 未绑定时回落到 {home}/workspaces/{sessionID}（会话隔离默认区，与文件面板同源，
	// 保证「Agent 写的文件 = 面板里看到的文件」）。
	wsResolver := tool.RootResolver(func(sessionID string) string {
		def := workspace
		if sessionID != "" && !strings.Contains(sessionID, "/") && !strings.Contains(sessionID, "\\") {
			def = filepath.Join(paths.Home, "workspaces", sessionID)
		}
		return sctx.WorkspaceRoot(ctx, sessionID, def)
	})
	// 文件变更追踪：file_write 写前落快照 + diff，前端可预览/回滚
	// 快照目录跟随会话工作区：绑定本地目录 → {dir}/.workbaby/snapshots/；默认 → {home}/snapshots/
	h.changeSvc = service.NewFileChangeService(
		h.app.FileChangeRepo, h.bus, filepath.Join(paths.Home, "snapshots"), workspace,
	).WithSnapshotRoot(func(sessionID string) string {
		_, sd := sctx.DataDirs(ctx, sessionID)
		return sd
	}).WithEmitter(emitter)
	// 工件登记：产出文件只记引用，前端经 /files 预览
	h.artifactSvc = service.NewArtifactService(
		h.app.ArtifactRepo, h.bus, workspace,
	).WithEmitter(emitter)
	toolReg := tool.NewRegistry()
	// 审批门：白名单外/危险命令 → 前端 chat:approval 事件确认后放行；暂停态持久化
	h.approvalSvc = service.NewApprovalService(h.bus).WithEmitter(emitter).WithRecords(h.app.ApprovalRecRepo).WithGrants(h.app.ApprovalGrantRepo)
	// exec 工具：白名单运行时动态读取（settings/exec/agent 设置页）；
	// cwd 缺省跟随会话工作区（绑定了外部目录时），命令与文件工具同一落点
	if err := toolReg.Register(exectool.New(tool.DefaultExecPolicy()).
		WithApprover(h.approvalSvc).
		WithRootResolver(tool.ResolveRoot(wsResolver, "")).
		// cwd 越界校验：LLM 显式传 cwd 时必须落在 workspace 根（含 .workbaby/ 子树）下，
		// 防过程脚本写到用户原有目录。未绑定工作区时 sandbox 为空字符串，exec 校验自动放行（向后兼容）。
		WithSandbox(func(ctx context.Context) string {
			wp := tool.ResolveRoot(wsResolver, "")(ctx)
			if wp == "" {
				return ""
			}
			return runtime.SandboxOf(wp).Root
		}).
		WithPathDirs(rt.BinDirs).
		WithWhitelist(func() []string {
			rows, err := h.app.SetRepo.ListAll(h.ctx)
			if err != nil {
				return []string{}
			}
			for _, r := range rows {
				if r.K == domain.SettingKeyExecWhitelist {
					var bins []string
					if json.Unmarshal([]byte(r.V), &bins) == nil {
						return bins
					}
					return []string{}
				}
			}
			return []string{}
		})); err != nil {
		return err
	}
	// Skill scripts 执行工具：SKILL.md scripts 挂成 Agent 可调用工具；
	// 脚本内容来自 skills 表，解释器固定映射，执行前过审批门
	if err := toolReg.Register(skillrun.New(
		func(skillName, scriptName string) (string, string, error) {
			return h.skillSvc.GetScript(h.ctx, skillName, scriptName)
		},
		workspace,
	).WithRootResolver(wsResolver).
		WithApprover(h.approvalSvc).
		WithPathDirs(rt.BinDirs)); err != nil {
		return err
	}
	if err := toolReg.Register(filetool.NewRead(wsResolver, workspace)); err != nil {
		return err
	}
	// 精确编辑：模型改文件的首选路径（唯一匹配校验 + diff 回执），file_write 退居新建/整篇覆盖
	if err := toolReg.Register(filetool.NewEdit(wsResolver, workspace).
		WithRecorder(service.NewFileChangeRecorder(h.changeSvc, h.artifactSvc, "file_edit"))); err != nil {
		return err
	}
	if err := toolReg.Register(filetool.NewWrite(wsResolver, workspace).
		WithRecorder(service.NewFileChangeRecorder(h.changeSvc, h.artifactSvc, "file_write"))); err != nil {
		return err
	}
	if err := toolReg.Register(filetool.NewList(wsResolver, workspace)); err != nil {
		return err
	}
	if err := toolReg.Register(filetool.NewGrep(wsResolver, workspace)); err != nil {
		return err
	}
	if err := toolReg.Register(filetool.NewGlob(wsResolver, workspace)); err != nil {
		return err
	}
	if err := toolReg.Register(webfetchtool.New()); err != nil {
		return err
	}
	if err := toolReg.Register(httptool.New()); err != nil {
		return err
	}
	if err := toolReg.Register(websearchtool.New()); err != nil {
		return err
	}
	// 纯函数工具（math/date/text/regex/json/csv/hash/encode/random）
	for _, ft := range functools.All() {
		if err := toolReg.Register(ft); err != nil {
			return err
		}
	}
	// 文档解析 + 归档工具（workspace 相对路径）
	if err := toolReg.Register(doctool.New(wsResolver, workspace)); err != nil {
		return err
	}
	if err := toolReg.Register(archivetool.New(wsResolver, workspace)); err != nil {
		return err
	}

	// 子 Agent 委派：上下文/预算隔离，只回传摘要
	if err := toolReg.Register(delegatetool.New()); err != nil {
		return err
	}

	// 补充输入：模型主动向用户要信息（暂停-回复-续跑）
	if err := toolReg.Register(requestinput.New()); err != nil {
		return err
	}

	// Todo 计划工具：会话内待办（长任务先列计划再逐步勾选）
	h.todoStore = service.NewSessionTodoStore(h.app.TodoRepo)
	if err := toolReg.Register(todotool.New(h.todoStore, func(ctx context.Context) string {
		return agent.SessionIDFromCtx(ctx)
	})); err != nil {
		return err
	}

	// 计划模式：enter/exit 工具 + 只读硬拦 Guard（exit 经审批门，拒绝则保持计划模式）
	planStore := planmode.NewStore(func(toolName string) bool {
		t, ok := toolReg.Get(toolName)
		if !ok {
			return false
		}
		m := tool.MetaOf(t)
		return m.ReadOnly || t.RiskLevel() == tool.RiskReadOnly
	})
	if err := toolReg.Register(planmode.NewEnter(planStore)); err != nil {
		return err
	}
	if err := toolReg.Register(planmode.NewExit(planStore, h.approvalSvc)); err != nil {
		return err
	}

	// 知识库 RAG：FTS5 检索 + 索引；本地导入文件复制到 {home}/knowledge 受管目录。
	// knowledge_search 工具由知识库能力统一暴露（见下方能力注册表）
	knowledgeRepo := h.app.KnowledgeDocRepo
	retriever := rag.NewFTS5Retriever(gdb)
	h.knowledgeSvc = service.NewKnowledgeService(
		knowledgeRepo,
		rag.NewIndexer(gdb, rag.DefaultChunker()),
		retriever,
		filepath.Join(paths.Home, "knowledge"),
	)

	if err := toolReg.SelfCheckSchema(); err != nil {
		return err
	}
	h.toolSvc = service.NewToolService(toolReg, h.app.SetRepo)
	h.dashSvc = service.NewDashboardService(h.app.DashboardRepo, h.app.UsageRepo, h.toolSvc)
	h.docsSvc = service.NewDocsService()

	// 长期记忆：单份 MEMORY.md + FTS5 索引，按会话隔离到 {home}/memory/{sessionID}/。
	h.memSvc = memory.NewService(filepath.Join(paths.Home, "memory.db"), h.app.DB)
	h.memProxy = service.NewMemoryService(h.memSvc)

	// Skill 系统：内置技能入库 + 全局技能目录同步（工作区目录在 run 前叠加）
	globalSkillDir := filepath.Join(paths.Home, "skills")
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		pkg.L.Warn("mkdir global skills dir failed", "dir", globalSkillDir, "err", err)
	}
	skillRepo := h.app.SkillRepo
	h.skillSvc = service.NewSkillService(skillRepo, skill.NewRegistry()).
		WithGlobalDir(globalSkillDir)
	if err := h.skillSvc.SyncBuiltin(ctx); err != nil {
		return err
	}
	// 全局目录同步失败只告警：技能缺失影响能力，不应阻断应用启动
	if err := h.skillSvc.SyncGlobal(ctx); err != nil {
		pkg.L.Warn("sync global skills dir failed", "dir", globalSkillDir, "err", err.Error())
	}

	// 自定义子智能体：agent_profiles 表物化进 harness 注册表（delegate_task / 会话切换消费）
	h.agentSvc = service.NewAgentProfileService(h.app.AgentProfileRepo).WithDataHome(paths.Home)
	if err := h.agentSvc.Sync(ctx); err != nil {
		pkg.L.Warn("sync agent profiles failed", "err", err.Error())
	}

	// 自定义斜杠命令：user_commands 表（/ 面板与设置页共用）
	h.commandSvc = service.NewUserCommandService(h.app.UserCommandRepo)

	// MCP：外部工具源；启动失败只标记 unready，不阻断
	mcpRepo := h.app.McpRepo
	h.mcpSvc = service.NewMcpService(mcpRepo, mcp.NewManager(toolReg).WithPathDirs(rt.BinDirs), h.cipher).
		WithConfigPath(service.McpRawPath(paths.Home))
	// mcp.json 文件为源：存在则同步进 mcp_servers 表后再对齐子进程
	if mcpPath := service.McpRawPath(paths.Home); isRegularFile(mcpPath) {
		if rows, merr := service.McpServersFromFile(mcpPath); merr != nil {
			pkg.L.Warn("sync mcp.json failed", "err", merr)
		} else if serr := h.mcpSvc.SyncMcpFromList(ctx, rows); serr != nil {
			pkg.L.Warn("sync mcp.json to db failed", "err", serr)
		}
	}
	if err := h.mcpSvc.Sync(ctx); err != nil {
		return err
	}

	// 目录信任：恒信任根 = 全局工作区 + 会话工作区根 + 数据目录本身。workspaces 整棵树
	// 纳入恒信任——它是 App 自己的受管区，纳入询问只会让每次写文件都弹审批。
	// exec 的 cwd 不在根内时走 ask → 审批门 → 批准后落盘 allow。
	trustRoots := []string{
		filepath.Join(paths.Home, "workspace"),
		filepath.Join(paths.Home, "workspaces"),
		filepath.Join(paths.Home, "runtimes"),
		filepath.Join(paths.Home, "knowledge"),
		filepath.Join(paths.Home, "files"),
	}
	h.trustSvc = service.NewTrustService(h.app.TrustRepo, trustRoots...).
		WithApprover(func(ctx context.Context, description string) bool {
			if h.approvalSvc == nil {
				return false
			}
			return h.approvalSvc.Approve(ctx, description, tool.RiskApprovalNeeds)
		})
	// 能力注册表：上下文装配（人格/工作区/记忆/知识库/技能）、工具暴露与
	// run 后沉淀统一接入；新增能力实现 Capability 并在此注册一行，chat 侧不再改动
	caps := capability.NewRegistry()
	registerCap := func(c capability.Capability, order int) {
		if err := caps.Register(c, order); err != nil {
			pkg.L.Warn("register capability failed", "cap", c.ID(), "err", err.Error())
		}
	}
	registerCap(capability.NewEnvironment(), agent.OrderEnv)
	registerCap(capability.NewWorkspace(), agent.OrderWorkspace)

	registerCap(capability.NewMemory(h.memSvc, func(c context.Context) bool {
		return service.MemoryEnabled(c, h.app.SetRepo)
	}), agent.OrderMemory)
	registerCap(capability.NewKnowledge(retriever), agent.OrderKnowledge)
	registerCap(capability.NewSkill(capability.NewSkillSource(
		func(input string) string { return h.skillSvc.Match(input) },
		func(name string) (capability.SkillHit, bool) {
			sk, ok := h.skillSvc.Get(name)
			if !ok || sk.Skill == nil {
				return capability.SkillHit{}, false
			}
			return capability.SkillHit{
				Name:        sk.Skill.Name,
				Body:        sk.Body,
				Tools:       sk.Tools,
				Source:      string(sk.Skill.SourceKind),
				Version:     sk.Skill.Version,
				Description: sk.Skill.Description,
			}, true
		},
		h.skillSvc.Summaries,
	)), agent.OrderSkill)
	// 能力暴露的工具统一注册（knowledge_search / memory_write）
	for _, t := range caps.Tools() {
		if err := toolReg.Register(t); err != nil {
			return err
		}
	}
	if err := toolReg.SelfCheckSchema(); err != nil {
		return err
	}
	// 文件系统：文件夹树 + 文件托管 + 会话工作区面板（面板与工具链共用会话目录解析）
	h.folderSvc = service.NewFolderService(h.app.FolderRepo)
	h.fileSvc = service.NewFileService(h.app.FileRepo, filepath.Join(paths.Home, "files"))
	h.workspaceSvc = service.NewWorkspaceService(filepath.Join(paths.Home, "workspaces"), func(sessionID string) string {
		return sctx.WorkspaceRoot(ctx, sessionID, filepath.Join(paths.Home, "workspaces", sessionID))
	})

	// 用户钩子：设置页管理 + run 生命周期（run_start/before_tool/after_tool/run_end）
	h.hookSvc = service.NewUserHookService(h.app.UserHookRepo)

	// ChatService：依赖一次性注入（ChatDeps），并以 MissingDeps 自检兜住漏接。
	h.chatSvc = service.NewChatService(service.ChatDeps{
		Sessions: h.app.SessRepo, Messages: h.app.MsgRepo, Providers: h.app.ProvRepo,
		Settings: h.app.SetRepo, Usages: h.app.UsageRepo,
		Bus: h.bus, Registry: h.reg, Tools: h.toolSvc, Memory: h.memSvc,
		Session:      sctx,
		Checkpoints:  service.NewSQLCheckpointStore(h.app.CheckpointRepo),
		EventLog:     h.eventLog,
		Emitter:      emitter,
		Blocks:       h.app.BlocksRepo,
		RunRecords:   h.app.RunRecRepo,
		Executions:   h.execs,
		Approvals:    h.approvalSvc,
		Trust:        h.trustSvc,
		Changes:      h.changeSvc,
		PlanStore:    planStore,
		Hooks:        h.hookSvc,
		Files:        h.fileSvc,
		Capabilities: caps,
		SkillSync:    h.skillSvc.SyncWorkspace,
	})
	if missing := h.chatSvc.MissingDeps(); len(missing) > 0 {
		return pkg.New(2003, "chat 装配不完整，缺少依赖", strings.Join(missing, ", "))
	}
	// durable pause：审批跨重启决策后从检查点续跑原 run。
	// approval ↔ chat 是真实循环依赖，保留唯一一处构造后绑定。
	h.approvalSvc.WithResumeHook(func(c context.Context, runID string) error {
		_, err := h.chatSvc.ResumeRun(c, runID)
		return err
	})

	// 启动恢复：遗留未决审批重武装决策窗口（决策即续跑，durable pause）；
	// 崩溃时卡在 streaming 的消息标 interrupted（可经 Resume 续跑）
	h.approvalSvc.RearmPending(ctx)
	h.approvalSvc.LoadGrants(ctx)
	h.chatSvc.ReapInterrupted(ctx)

	// 后台任务域：提交/列表/取消 + task:* SSE 事件（worker 池随构造启动）。
	// 必须在 chatSvc 全部装配完成之后构造：任务执行复用其工具注册表/护栏链/用量落账。
	h.taskSvc = service.NewChatTaskService(h.app.ChatTaskRepo, h.bus, h.chatSvc, h.approvalSvc, h.app.SessRepo)

	h.ctx = ctx
	h.Files = &FileServer{
		ctx:          ctx,
		fileSvc:      h.fileSvc,
		workspaceSvc: h.workspaceSvc,
	}
	h.bindEventBridge()
	// app:ready 由 main.go 在 gin server 启动后发射
	sl, lerr := h.skillSvc.List(ctx)
	ml, merr := h.mcpSvc.List(ctx)
	fields := []any{"home", paths.Home, "cipherReady", true, "tools", len(toolReg.List())}
	if lerr == nil {
		fields = append(fields, "skills", len(sl))
	}
	ready := 0
	if merr == nil {
		for i := range ml {
			if ml[i].Ready {
				ready++
			}
		}
		fields = append(fields, "mcpServers", len(ml), "mcpReady", ready)
	}
	pkg.L.Info("workbaby started", fields...)
	return nil
}

// Shutdown 回收 MCP 子进程。
func (h *Handler) Shutdown(_ context.Context) {
	if h.mcpSvc != nil {
		h.mcpSvc.Close() // 回收 MCP 子进程，避免残留孤儿进程
	}
	if pkg.L != nil {
		pkg.L.Info("workbaby shutting down")
	}
}

// bindEventBridge 应用内事件总线 → Wails 前端事件总线（仅系统级事件 app:*）。chat:* 不经
// 此桥接：前端统一走 SSE，Wails 通道零订阅者，而每条增量都会在 run goroutine 上多做一次
// 序列化与 IPC（纯开销）。
func (h *Handler) bindEventBridge() {
	h.bus.Subscribe(event.MatchPrefix("app:"), func(event string, payload any) {
		if h.ctx == nil {
			return
		}
		wruntime.EventsEmit(h.ctx, event, payload)
	})
}
