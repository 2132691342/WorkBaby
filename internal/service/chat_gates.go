package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"WorkBaby/internal/capability"
	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/tool/planmode"
)

// 本文件：装配与权限（With* 注入 / 工具门 / 目录信任 / 计划模式 / 轮间调整 / 事件发射）。

func (s *ChatService) WithCapabilities(caps *capability.Registry) *ChatService {
	s.caps = caps
	return s
}

// WithExecutionRegistry 启用执行平面：登记 run 的 scope/state，供统一执行拓扑查询。
func (s *ChatService) WithExecutionRegistry(reg *core.ExecutionRegistry) *ChatService {
	s.execs = reg
	return s
}

func (s *ChatService) WithCheckpointStore(store core.CheckpointStore) *ChatService {
	s.checkpoints = store
	return s
}

// WithEventLog 启用 run 事件日志：每条事件分配单调序号并缓存，SSE 断线可重放。
func (s *ChatService) WithEventLog(log *event.RunEventLog) *ChatService { s.events = log; return s }

// WithMessageBlocks 启用消息块持久化：工具调用/结果/产物随事件落 message_blocks，
// 刷新或切会话后历史消息可复现完整工具过程。
func (s *ChatService) WithMessageBlocks(r *repo.MessageBlockRepo) *ChatService {
	s.blocks = r
	return s
}

// WithApprovalService 注入审批服务：工具策略门 ask 决策经 chat:approval 事件等用户回执。
func (s *ChatService) WithApprovalService(a *ApprovalService) *ChatService { s.approval = a; return s }

// WithTrustService 注入目录信任；仅在 harness 装配 PathTrust 时才生效。
func (s *ChatService) WithTrustService(t *TrustService) *ChatService { s.trust = t; return s }

// WithFileStore 注入附件读取能力（图片 → data URI）；nil 时消息附件只降级为文本提示。
func (s *ChatService) WithFileStore(f FileStore) *ChatService { s.files = f; return s }

// WithChangeService 注入文件变更服务（完成度证据：核对声明路径与本 run file_changes）。
func (s *ChatService) WithChangeService(c *FileChangeService) *ChatService { s.changeSvc = c; return s }

// WithDataHome 注入数据根（paths.Home）；目录策略（记忆/快照默认根）由此派生。
// 必须在装配会话级目录解析闭包前调用。
func (s *ChatService) WithDataHome(home string) *ChatService {
	s.dataHome = home
	return s
}

// WithSkillSync 注入技能目录同步钩子：run 前按会话工作区叠加技能。
func (s *ChatService) WithSkillSync(fn func(context.Context, string) error) *ChatService {
	s.skillSync = fn
	return s
}

func (s *ChatService) memoryEnabled(ctx context.Context) bool {
	if s.setRepo == nil {
		return true
	}
	row, err := s.setRepo.Get(ctx, domain.SettingKeyMemoryEnabled)
	if err != nil || row == nil {
		return true
	}
	return strings.TrimSpace(row.V) != "false"
}

// MemoryEnabled 记忆全局开关（能力装配方回调用）。
func (s *ChatService) MemoryEnabled(ctx context.Context) bool { return s.memoryEnabled(ctx) }

// SessionDataDirs 某会话的记忆文件与快照目录（目录策略唯一数据源）。
// 绑定本地工作区 → {dir}/.workbaby/ 下（过程数据跟工作区走）；未绑定 → {dataHome} 下。
func (s *ChatService) SessionDataDirs(ctx context.Context, sessionID string) (memoryFile, snapshotDir string) {
	home := s.dataHome
	if home == "" {
		home = "."
	}
	memoryFile = filepath.Join(home, "memory", sessionID, "MEMORY.md")
	snapshotDir = filepath.Join(home, "snapshots", sessionID)
	if sessionID == "" || s.sessions == nil {
		return
	}
	row, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil || row == nil {
		return
	}
	if wp := strings.TrimSpace(row.WorkspacePath); wp != "" {
		sb := runtime.SandboxOf(wp)
		memoryFile = filepath.Join(sb.Memory, sessionID, "MEMORY.md")
		snapshotDir = filepath.Join(sb.Snapshots, sessionID)
	}
	return
}

// gateInternalAllowTools 工具策略门显式放行清单（default 模式下本会走 ask 的那些）。
//
// 放行不等于无护栏：实现了 tool.RiskClassifier 的工具（exec / run_skill_script）仍由
// runner 拿 ClassifyArgs 的 per-call 风险做命令级裁决——白名单外与危险正则命中照样弹审批。

var gateInternalAllowTools = []string{
	"exec", "run_skill_script", "delegate_task", // 命令级裁决走 RiskClassifier
	"websearch", "webfetch", "http", // 信息型网络读取
	"knowledge_search", "doc_reader", "todo", // 只读 / 会话内计划
	"file_write", "archive_manager", "memory_write", // 工作区沙箱内的本地写
}

// WithPlanStore 注入计划模式状态（enter/exit_plan_mode 工具与 Guard 共用同一 store）。
func (s *ChatService) WithPlanStore(p *planmode.Store) *ChatService {
	s.planStore = p
	return s
}

// extractTargetDir 从工具入参抽取目标目录。
//
// 返回 (dir, ok)；ok=false 表示该工具与目录无关，调用方应跳过信任检查。
func extractTargetDir(toolName string, args json.RawMessage) (string, bool) {
	switch toolName {
	case "exec":
		// cwd 优先；为空 → 走进程默认目录（app home），跳过信任检查
		var v struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(args, &v); err != nil {
			return "", true
		}
		return v.Cwd, true
	case "file_read", "file_write", "file_list", "doc_reader":
		// 这些工具已被工作区根沙箱约束，目录信任在工具内部处理
		return "", false
	case "archive":
		// archive 的 source/target 是工作区相对路径，不涉及外部目录
		return "", false
	default:
		return "", false
	}
}

// todoToolName 计划工具名；其结构化产出单独发 chat:todo 事件（前端进度卡）。

const todoToolName = "todo"

// emit 发布 run 事件：注入 run_id/session_id、经事件日志分配 seq，再广播到总线。
// 载荷字段一律 snake_case（CLAUDE.md §2.5）。
func (s *ChatService) emit(runID, sessionID, name string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["run_id"] = runID
	payload["session_id"] = sessionID
	if s.events != nil {
		s.events.Append(runID, name, payload)
	}
	s.bus.Publish(name, payload)
}

// NewChatService 注入 repo、Registry、工具、记忆与 Skill 服务。
