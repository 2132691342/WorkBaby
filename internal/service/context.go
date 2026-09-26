package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"WorkBaby/internal/capability"
	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/modelmeta"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/resource"
	"WorkBaby/internal/runtime"
)

// defaultContextWindow provider 未声明上下文窗口时的兜底值（与前端 ChatView 的 128000 一致）。
const defaultContextWindow = 128_000

// runtimeHome 数据目录的包级间接层：便于单测替换（生产走 runtime.Resolve）。
var runtimeHome = func() string {
	paths, err := runtime.Resolve()
	if err != nil {
		return ""
	}
	return paths.Home
}

// ContextUsage 会话上下文占用分段快照（/context）。
// 与真实请求同源（buildSystem）；recall 用会话最后一条用户消息作检索代理；
// 历史段优先用末条 assistant 的实测 input_tokens，拿不到按估算并置 Estimated。
func (s *ChatService) ContextUsage(ctx context.Context, sessionID string) (domain.ContextUsageRESP, error) {
	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return domain.ContextUsageRESP{}, err
	}
	resp := domain.ContextUsageRESP{
		SessionID:     ses.ID,
		Model:         ses.Model,
		ContextWindow: s.modelContextWindow(ctx, ses.ProviderID, ses.Model),
	}
	provRow := s.providerDO(ctx, ses.ProviderID)
	resp.ContextBudget = s.contextBudget(ctx, resp.ContextWindow, provRow)

	// history：实测优先，估算兜底（先取 rows，recall 代理要从里面找最后一条用户输入）
	rows, err := s.messages.ListBySession(ctx, ses.ID, 0, 0)
	if err != nil {
		return domain.ContextUsageRESP{}, err
	}
	history, measured := historyTokens(rows)
	if !measured {
		msgs, err := s.toLLMMessages(rows)
		if err != nil {
			return domain.ContextUsageRESP{}, err
		}
		history = agent.EstimateTokens(msgs)
	}
	resp.Estimated = !measured

	// system：与真实请求同一装配入口；recall 用最后一条用户消息作检索代理
	def := agent.Agent(defaultAgentName)
	system := ""
	if sys, _ := s.buildSystem(ctx, ses, "preview", lastUserInput(rows), def); sys != nil {
		system = sys.Content
	}

	// tools：与真实请求同一入口（skillTools 传 nil = 无技能命中），保证透视口径等于实发口径
	toolDefs := s.exposedToolDefs(ctx, def, nil)
	toolsJSON, err := json.Marshal(toolDefs)
	if err != nil {
		toolsJSON = nil
	}

	// memory 段已并入 system（buildSystem 的记忆能力注入就是真实请求里的 memory
	// 正文；单列会在透视里重复计数）
	resp.Segments = []domain.ContextSegment{
		{Key: "system", Title: "系统提示（含记忆注入）", Tokens: pkg.EstimateTextTokens(system)},
		{Key: "tools", Title: "工具定义", Tokens: pkg.EstimateTextTokens(string(toolsJSON))},
		{Key: "history", Title: "历史消息", Tokens: history},
	}
	for _, seg := range resp.Segments {
		resp.UsedTokens += seg.Tokens
	}
	for i := range resp.Segments {
		resp.Segments[i].Ratio = perMille(resp.Segments[i].Tokens, resp.ContextWindow)
	}
	resp.FreeTokens = resp.ContextWindow - resp.UsedTokens
	if resp.FreeTokens < 0 {
		resp.FreeTokens = 0
	}
	resp.UsedRatio = perMille(resp.UsedTokens, resp.ContextWindow)
	resp.MessageCount = len(rows)
	resp.ToolCount = len(toolDefs)
	return resp, nil
}

// lastUserInput 取最后一条用户消息正文（recall 检索代理）；无则空串。
func lastUserInput(rows []domain.MessageDO) string {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Role == domain.MessageRoleUser && strings.TrimSpace(rows[i].Content) != "" {
			return rows[i].Content
		}
	}
	return ""
}

// contextWindow 取 provider 声明的上下文窗口；provider 缺失/取失败返回 0（调用方兜底）。
func (s *ChatService) contextWindow(ctx context.Context, providerID string) int {
	if s.provRepo == nil || providerID == "" {
		return 0
	}
	row, err := s.provRepo.GetByID(ctx, providerID)
	if err != nil || row == nil {
		return 0
	}
	return row.ContextWindow
}

// modelContextWindow 上下文窗口三级回退：provider 声明 → 内置模型目录 → 全局缺省。
func (s *ChatService) modelContextWindow(ctx context.Context, providerID, model string) int {
	if w := s.contextWindow(ctx, providerID); w > 0 {
		return w
	}
	if w := modelmeta.ContextWindow(model); w > 0 {
		return w
	}
	return defaultContextWindow
}

// historyTokens 取末条 assistant 落库的实测 input_tokens；没有实测值时 measured=false。
func historyTokens(rows []domain.MessageDO) (tokens int, measured bool) {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Role == domain.MessageRoleAssistant && rows[i].InputTokens > 0 {
			return rows[i].InputTokens, true
		}
	}
	return 0, false
}

// perMille 千分比（避免前端浮点误差；分母 <=0 返回 0）。
func perMille(part, total int) int {
	if total <= 0 {
		return 0
	}
	return part * 1000 / total
}

// 拼接顺序：能力注入段由注册序给出（最大 OrderSkill=60），附加段排在其后。
// 顺序决定正文里的出现位置，Priority 决定预算紧张时谁先走——两者必须分开设，
// 否则「最后注入的工作区沙箱」会因为排在末尾而被优先丢掉。
const (
	orderWorkspaceSandbox = 71
	orderAgentsMd         = 75
	orderArchiveSummary   = 76
	orderCompactHint      = 77
	orderVerifyHint       = 78
)

// buildSystem 装配本轮 system 段；与 /context 占用透视同源，保证「看到的占用」等于「实际发送」。
// 两段来源：能力注册表注入段（PreloadAll 按 order 串联）+ 服务侧附加段（需读仓储与会话状态）。
func (s *ChatService) buildSystem(ctx context.Context, ses *domain.ChatSessionDO, runID, userInput string, def agent.Definition) (*llm.Message, *capability.RunState) {
	state := &capability.RunState{}
	sections := s.caps.PreloadAll(ctx, &capability.PreloadCtx{
		SessionID: ses.ID,
		RunID:     runID,
		UserInput: userInput,
		Session:   ses,
		Def:       def,
		State:     state,
	})
	sections = append(sections, s.extraSystemSections(ctx, ses)...)

	// system 段预算：上下文 token 预算 × 30%（rune≈0.6 token，中英混合口径）。
	// system 被压缩器无条件保留，没有上限会只增不减。
	sysRunes := 0
	if window := s.contextWindow(ctx, ses.ProviderID); window > 0 {
		if b := s.contextBudget(ctx, window, s.providerDO(ctx, ses.ProviderID)); b > 0 {
			sysRunes = b * 3 / 10 * 2
		}
	}
	body, dropped := agent.BuildSystem(sections, sysRunes)
	if len(dropped) > 0 {
		pkg.L.Info("system pieces dropped by budget", "sessionID", ses.ID, "runID", runID, "dropped", strings.Join(dropped, ","))
		// 裁剪必须回传前端：用户看到的回答质量下降（如 Skill 正文被丢）需要能对上原因。
		// runID 为空是「占用透视」路径（不产生 run 事件），跳过。
		if runID != "" {
			s.emit(runID, ses.ID, "chat:context-trimmed", map[string]any{
				"dropped_segments": dropped,
				"budget_runes":     sysRunes,
			})
		}
	}
	if body == "" {
		return nil, state
	}
	return llm.SystemMessage(body), state
}

// extraSystemSections 服务侧附加段：能力注入之外、需要读仓储或会话状态的 system 内容。
// 拼接序 = Order（决定正文出现位置），丢弃序 = Priority（预算紧张时谁先走），两者必须分开设。
func (s *ChatService) extraSystemSections(ctx context.Context, ses *domain.ChatSessionDO) []agent.Section {
	var sections []agent.Section
	// 项目指令（AGENTS.md）：用户全局 + 工作区两份，先全局后工作区。
	// 指令文件承载稳定约定（技术栈 / 规范 / 验证方式），每轮 run 都注入 system。
	// 加载逻辑在 internal/resource（PI Phase 6 资源统一入口）。
	sections = append(sections, resource.LoadAgentsMd(runtimeHome(), strings.TrimSpace(ses.WorkspacePath))...)
	// 历史归档摘要：/compact 归档掉的早期轮次的确定性摘要（模型据此知道
	// 被剔除的历史讲了什么，而不是凭空失忆）
	if sum := archiveSummary(ses); sum != "" {
		sections = append(sections, agent.Section{
			Key: "archive", Title: "历史归档摘要", Body: sum,
			Order: orderArchiveSummary, Priority: agent.PriorityHigh,
		})
	}
	// 压缩保留指示：写进会话元数据，每次 run 都装进 system——不受历史折叠影响
	if ins := compactInstructions(ses); ins != "" {
		sections = append(sections, agent.Section{
			Key: "compact", Title: "压缩保留指示", Body: ins,
			Order: orderCompactHint, Priority: agent.PriorityHigh,
		})
	}
	// 验证提醒：上一轮修改了文件但没执行任何命令 → 注入提醒。
	// 改了代码却直接交付是「任务跑不完整」的常见根因。
	if hint := s.verificationReminder(ctx, ses.ID); hint != "" {
		sections = append(sections, agent.Section{
			Key: "verify", Title: "验证提醒", Body: hint,
			Order: orderVerifyHint, Priority: agent.PriorityLow,
		})
	}
	// 工作区沙箱强制隔离：未绑定外部工作区时不注入，避免空谈约束。
	// 已绑定则给 LLM 明确的 .workbaby/ 子目录路径与「工作区根只读」红线——
	// 解决把过程脚本直接落到工作区根、污染用户原有目录的问题。
	if wp := strings.TrimSpace(ses.WorkspacePath); wp != "" {
		sb := runtime.SandboxOf(wp)
		sections = append(sections, agent.Section{
			Key:   "workspace_sandbox",
			Title: "工作区沙箱（强制隔离）",
			Body: fmt.Sprintf(
				"工作区 %s 下已建立 .workbaby/ 目录树；所有过程数据**必须**写入下列子目录，"+
					"否则视为污染用户原有目录结构：\n"+
					"  - 脚本：%s\n"+
					"  - 产出：%s\n"+
					"  - 缓存：%s\n"+
					"  - 临时：%s\n"+
					"工作区根目录视为只读输入源，禁止在 .workbaby/ 之外创建脚本或写入产物。",
				wp, sb.Scripts, sb.Output, sb.Cache, sb.Tmp,
			),
			Order:    orderWorkspaceSandbox,
			Priority: agent.PriorityEssential,
		})
	}
	return sections
}

// verificationReminder 验证提醒：最近一条 assistant 轮次修改了文件但未执行任何命令 →
// 返回提醒文案（下一轮注入 system）。纯只读判断，无修改/已验证返回空串。
func (s *ChatService) verificationReminder(ctx context.Context, sessionID string) string {
	if s.messages == nil {
		return ""
	}
	rows, err := s.messages.ListRecentBySession(ctx, sessionID, 20)
	if err != nil {
		return ""
	}
	var edited, executed bool
	for _, m := range rows {
		if m.Role != domain.MessageRoleAssistant || m.ToolCalls == "" {
			continue
		}
		var calls []llm.ToolCall
		if json.Unmarshal([]byte(m.ToolCalls), &calls) != nil {
			continue
		}
		// 只看最近一个含工具调用的 assistant 轮
		for _, c := range calls {
			switch c.Function.Name {
			case "file_edit", "file_write", "archive_manager":
				edited = true
			case "exec", "run_skill_script":
				executed = true
			}
		}
		break
	}
	if !edited || executed {
		return ""
	}
	return "上一轮修改了文件但没有运行任何验证命令。" +
		"在继续或交付前，先用 exec 跑一次最小验证（编译 / 测试 / 目标脚本），确认改动真实可用，再向用户汇报结果。"
}

// skillBlockPayload 技能命中载荷：chat:skill 事件与 skill 消息块共用同一份（字段 snake_case）。
func skillBlockPayload(st *capability.RunState) map[string]any {
	return map[string]any{
		"name":           st.SkillName,
		"source":         st.SkillSource,
		"version":        st.SkillVersion,
		"description":    st.SkillDescription,
		"tools":          st.SkillTools,
		"injected_chars": st.SkillInjectedLen,
	}
}

// toLLMMessages 历史消息 → llm.Message，重建 assistant 工具调用与 tool 消息上下文（无附件展开，
// 供上下文占用透视等非 run 路径复用）。过滤孤儿 tool 消息与空 assistant 占位——不剥掉上游会 400。
func (s *ChatService) toLLMMessages(hists []domain.MessageDO) ([]*llm.Message, error) {
	return s.buildLLMMessages(context.Background(), hists, false)
}

// toLLMMessagesWithVision vision=true 时把最后一条用户消息的图片附件转成多模态 part
// （历史轮次的图片只降级为文本提示，避免老图片反复吃 token）。
func (s *ChatService) toLLMMessagesWithVision(ctx context.Context, hists []domain.MessageDO, vision bool) ([]*llm.Message, error) {
	return s.buildLLMMessages(ctx, hists, vision)
}

// includeInLLMContext 消息是否进入 LLM 上下文（消息分层的唯一判定入口）。
// 排除 streaming 占位、archived 归档轮次，以及 ContextScope=ui 的界面消息。
func includeInLLMContext(m domain.MessageDO) bool {
	if m.Status == domain.MessageStatusStreaming || m.Status == domain.MessageStatusArchived {
		return false
	}
	return m.ContextScope != domain.MessageScopeUI
}

func (s *ChatService) buildLLMMessages(ctx context.Context, hists []domain.MessageDO, vision bool) ([]*llm.Message, error) {
	lastUser := -1
	for i := range hists {
		if hists[i].Role == domain.MessageRoleUser {
			lastUser = i
		}
	}
	knownToolIDs := make(map[string]struct{}, len(hists))
	for _, m := range hists {
		if m.Role != domain.MessageRoleAssistant || m.ToolCalls == "" {
			continue
		}
		var calls []llm.ToolCall
		if err := json.Unmarshal([]byte(m.ToolCalls), &calls); err != nil {
			continue
		}
		for _, c := range calls {
			if c.ID != "" {
				knownToolIDs[c.ID] = struct{}{}
			}
		}
	}
	out := make([]*llm.Message, 0, len(hists))
	answeredToolIDs := make(map[string]struct{}, len(hists))
	for _, m := range hists {
		if m.Role == domain.MessageRoleTool && m.ToolCallID != "" {
			answeredToolIDs[m.ToolCallID] = struct{}{}
		}
	}
	for i := range hists {
		m := hists[i]
		if !includeInLLMContext(m) {
			continue
		}
		if m.Role == domain.MessageRoleTool {
			if m.ToolCallID == "" {
				continue
			}
			if _, ok := knownToolIDs[m.ToolCallID]; !ok {
				continue
			}
		}
		lm := &llm.Message{Role: llm.RoleType(m.Role), Content: m.Content, Thinking: m.Thinking}
		if m.Role == domain.MessageRoleUser {
			s.applyAttachments(ctx, lm, &m, i == lastUser && vision)
		}
		if m.Role == domain.MessageRoleTool {
			lm.ToolCallID = m.ToolCallID
			// 空 tool 结果兜底：GLM 等厂商对空 content 一律 400（1214）；剥掉会破坏配对，故填充
			if lm.Content == "" {
				lm.Content = "(empty)"
			}
		}
		if m.Role == domain.MessageRoleAssistant && m.ToolCalls == "" && lm.Content == "" {
			// 失败/中断 run 残留的空 assistant 占位：上游以 400「messages 参数非法」
			// 拒绝整轮（GLM 上游码 1214），且已落库会永久毒化会话——静默剥掉
			continue
		}
		if m.Role == domain.MessageRoleAssistant && m.ToolCalls != "" {
			var calls []llm.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &calls); err != nil {
				return nil, pkg.Wrap(2062, "parse tool_calls failed", err)
			}
			// 悬挂 tool_calls 剥离（取消/崩溃残段）：assistant 发起了调用但结果未落库，
			// 原样回发上游是协议违规（tool_use 无对应 tool_result 必 400）。
			// 只剥调用不剥正文——半截回答保留进上下文，中断后续聊叙事不断裂。
			kept := make([]llm.ToolCall, 0, len(calls))
			for _, c := range calls {
				if _, ok := answeredToolIDs[c.ID]; ok {
					kept = append(kept, c)
				}
			}
			if len(kept) > 0 {
				lm.ToolCalls = kept
			} else if lm.Content == "" && lm.Thinking == "" {
				// 调用全部悬挂且无正文：等价空占位，剥掉
				continue
			}
		}
		out = append(out, lm)
	}
	return out, nil
}

// applyAttachments 把消息附件挂到 LLM 消息上：图片 + 模型支持视觉 → 多模态 part；
// 其余（含不支持视觉时的图片）降级为文本行，保证模型至少知道用户附了什么。
func (s *ChatService) applyAttachments(ctx context.Context, lm *llm.Message, m *domain.MessageDO, vision bool) {
	atts := m.Attachments()
	if len(atts) == 0 {
		return
	}
	var lines []string
	for _, a := range atts {
		if vision && a.Kind == domain.AttachmentImage && s.files != nil {
			u, err := s.files.ReadDataURL(ctx, a.ID)
			if err == nil && u != "" {
				lm.Parts = append(lm.Parts, llm.ContentPart{Type: "image_url", ImageURL: &llm.ImageURL{URL: u}})
				continue
			}
		}
		lines = append(lines, "[附件] "+a.Name+"（"+a.MIME+"）")
	}
	if len(lines) == 0 {
		return
	}
	note := strings.Join(lines, "\n")
	if lm.Content == "" {
		lm.Content = note
		return
	}
	lm.Content += "\n" + note
}

// providerVision 会话所用 Provider 是否支持视觉（显式声明 > 模型名推断）。
func (s *ChatService) providerVision(ctx context.Context, providerID string) bool {
	if providerID == "" {
		return false
	}
	p, err := s.provRepo.GetByID(ctx, providerID)
	if err != nil {
		return false
	}
	return p.SupportsVisionEffective()
}

// toolResultContent 组装落库的 tool 消息正文（含错误信息）。

func (s *ChatService) providerParams(ctx context.Context, providerID string) *llm.ProviderParams {
	if row := s.providerDO(ctx, providerID); row != nil {
		return llm.ProviderParamsFromDO(row)
	}
	return nil
}

// providerDO 取 Provider 实体；缺失/查失败返回 nil（调用方各自兜底，不因配置缺失中断 run）。
func (s *ChatService) providerDO(ctx context.Context, providerID string) *domain.AiProviderDO {
	if s.provRepo == nil || providerID == "" {
		return nil
	}
	row, err := s.provRepo.GetByID(ctx, providerID)
	if err != nil || row == nil {
		return nil
	}
	return row
}

// defaults 从 system_settings 读 chat 默认温度/思考；读失败回退到内置默认（0.25 / topP 0.75 / medium）。
// topP 暂无独立设置项：它是与 temperature 联动的采样旋钮，单独暴露只会增加配置面。
func (s *ChatService) defaults(ctx context.Context) llm.Defaults {
	d := llm.Defaults{
		Temperature: defaultTemperature,
		TopP:        defaultTopP,
		Thinking:    llm.ThinkingFromEffort("medium"),
	}
	if s.setRepo == nil {
		return d
	}
	if row, err := s.setRepo.Get(ctx, domain.SettingKeyChatDefaultTemperature); err == nil && row != nil {
		if t, perr := strconv.ParseFloat(strings.TrimSpace(row.V), 64); perr == nil {
			d.Temperature = t
		}
	}
	if row, err := s.setRepo.Get(ctx, domain.SettingKeyChatDefaultThinking); err == nil && row != nil {
		if tc := llm.ThinkingFromEffort(strings.TrimSpace(row.V)); tc != nil {
			d.Thinking = tc
		}
	}
	return d
}

// 采样与上下文相关默认值（system_settings / Provider 未配置时的兜底）。
const (
	defaultTemperature      = 0.25  // 默认采样温度
	defaultTopP             = 0.75  // 默认核采样阈值
	defaultCompressionRatio = 0.75  // 上下文占用达窗口 75% 触发压缩
	defaultMaxInputChars    = 32000 // 单条用户输入上限（含附件展开文本）
)

// settingFloat 读数值型系统设置；缺失/非法返回 fallback。
func (s *ChatService) settingFloat(ctx context.Context, key string, fallback float64) float64 {
	if s.setRepo == nil {
		return fallback
	}
	row, err := s.setRepo.Get(ctx, key)
	if err != nil || row == nil {
		return fallback
	}
	v, perr := strconv.ParseFloat(strings.TrimSpace(row.V), 64)
	if perr != nil {
		return fallback
	}
	return v
}

// compressionRatio 压缩触发比例：Provider 级 compress_ratio 优先，其次全局 chat.compressionRatio。
func (s *ChatService) compressionRatio(ctx context.Context, prov *domain.AiProviderDO) (ratio float64, from string) {
	if prov != nil && prov.CompressRatio > 0 && prov.CompressRatio <= 1 {
		return prov.CompressRatio, "provider"
	}
	return s.settingFloat(ctx, domain.SettingKeyChatCompressionRatio, defaultCompressionRatio), "default"
}

// contextBudget 本轮上下文预算：上下文窗口 × 压缩比例；压缩器在估算 token 超此值时触发。
func (s *ChatService) contextBudget(ctx context.Context, window int, prov *domain.AiProviderDO) int {
	ratio, _ := s.compressionRatio(ctx, prov)
	if window <= 0 {
		return 0
	}
	return int(float64(window) * ratio)
}

// EffectiveParams 当前会话实际生效参数快照：合并「Provider 级 → 全局默认 → 内置兜底」三层，
// 并标注每层来源。前端输入框与设置页共用此接口，杜绝两处显示不一致。
func (s *ChatService) EffectiveParams(ctx context.Context, sessionID string) (*domain.EffectiveParamsRESP, error) {
	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	prov := s.providerDO(ctx, ses.ProviderID)
	def := s.defaults(ctx)
	// 思考强度按字符串透传（effort 是前端与 Provider 表共用的语义值，无需经 ThinkingConfig 反解）
	effort, effortFrom := "medium", "builtin"
	if s.setRepo != nil {
		if row, err := s.setRepo.Get(ctx, domain.SettingKeyChatDefaultThinking); err == nil && row != nil {
			if v := strings.TrimSpace(row.V); v != "" {
				effort, effortFrom = v, "default"
			}
		}
	}

	out := &domain.EffectiveParamsRESP{
		SessionID:         ses.ID,
		ProviderID:        ses.ProviderID,
		Model:             ses.Model,
		Temperature:       def.Temperature,
		TemperatureFrom:   "default",
		ThinkingEffort:    effort,
		ThinkingFrom:      effortFrom,
		ContextWindow:     defaultContextWindow,
		ContextWindowFrom: "builtin",
	}
	if prov != nil {
		if prov.Temperature > 0 {
			out.Temperature = prov.Temperature
			out.TemperatureFrom = "provider"
		}
		if prov.ThinkingEffort != "" {
			out.ThinkingEffort = prov.ThinkingEffort
			out.ThinkingFrom = "provider"
		}
		if prov.ContextWindow > 0 {
			out.ContextWindow = prov.ContextWindow
			out.ContextWindowFrom = "provider"
		}
	}
	ratio, ratioFrom := s.compressionRatio(ctx, prov)
	out.CompressionRatio = ratio
	out.CompressionFrom = ratioFrom
	out.ContextBudget = s.contextBudget(ctx, out.ContextWindow, prov)
	out.MaxInputChars = int(s.settingFloat(ctx, domain.SettingKeyChatMaxInputChars, defaultMaxInputChars))
	return out, nil
}
