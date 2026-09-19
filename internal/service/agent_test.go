package service

// Agent 装配链路测试：定义文件解析 → 注册表同步 → 自定义档案生命周期 → core.Loop 接线。
//
// 覆盖的是「配置怎么变成一次真实 run」这条链路：定义文件的解析与拒绝规则、目录级加载容错、
// 模型与推理强度的映射校验、档案增删改对注册表的同步、以及内核装配后护栏与事件映射是否接通。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/tool"
)

// ===== 测试替身 =====

// stubProvider 按调用序返回预设响应的测试 Provider。
type stubProvider struct {
	mu    sync.Mutex
	turns []llm.ChatResponse
	calls int
}

func (p *stubProvider) Name() string                                    { return "stub" }
func (p *stubProvider) Kind() llm.ProviderKind                          { return "openai" }
func (p *stubProvider) Models(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (p *stubProvider) Ping(context.Context) error                      { return nil }

func (p *stubProvider) Chat(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.calls
	p.calls++
	if i < len(p.turns) {
		r := p.turns[i]
		return &r, nil
	}
	return &llm.ChatResponse{}, nil
}

func (p *stubProvider) Stream(ctx context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	p.mu.Lock()
	i := p.calls
	p.calls++
	var resp llm.ChatResponse
	if i < len(p.turns) {
		resp = p.turns[i]
	}
	p.mu.Unlock()

	out := make(chan llm.StreamChunk)
	send := func(c llm.StreamChunk) bool {
		select {
		case out <- c:
			return true
		case <-ctx.Done():
			return false
		}
	}
	go func() {
		defer close(out)
		if resp.Message.Content != "" {
			if !send(llm.StreamChunk{Delta: llm.Message{Role: llm.RoleAssistant, Content: resp.Message.Content}}) {
				return
			}
		}
		for _, tc := range resp.ToolCalls {
			c := tc
			if !send(llm.StreamChunk{ToolCall: &c}) {
				return
			}
		}
		sr := resp.StopReason
		_ = send(llm.StreamChunk{FinishReason: &sr})
	}()
	return out, nil
}

// stubTool 测试工具。
type stubTool struct {
	mu    sync.Mutex
	name  string
	risk  tool.RiskLevel
	calls int
}

func (t *stubTool) Name() string        { return t.name }
func (t *stubTool) Description() string { return "stub " + t.name }

func (t *stubTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{Name: t.name, Description: "stub", Parameters: json.RawMessage(`{"type":"object"}`)}
}

func (t *stubTool) RiskLevel() tool.RiskLevel { return t.risk }

func (t *stubTool) Execute(context.Context, json.RawMessage) tool.ToolResult {
	t.mu.Lock()
	t.calls++
	t.mu.Unlock()
	return tool.ToolResult{Content: "ok:" + t.name}
}

func (t *stubTool) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

// ===== 内核装配接线 =====

// TestCoreLoopAssembly 装配层接线：真实护栏链 + 事件映射在装配后是否接通。
func TestCoreLoopAssembly(t *testing.T) {
	t.Run("跑通一次带工具的 ReAct 并映射事件", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, _ := seedSession(t, svc, msgRepo, ctx)

		echo := &stubTool{name: "echo", risk: tool.RiskReadOnly}
		require.NoError(t, svc.tools.Registry().Register(echo))

		prov := &stubProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, StopReason: "end_turn"},
		}}

		var mu sync.Mutex
		var names []string
		svc.bus.Subscribe(event.MatchPrefix("chat:"), func(name string, _ any) {
			mu.Lock()
			defer mu.Unlock()
			names = append(names, name)
		})

		mapper := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		loop := svc.newCoreLoop(coreLoopSpec{
			Ses:            &domain.ChatSessionDO{ID: ses.ID},
			RunID:          "RUN_1",
			AssistantMsgID: "MSG_1",
			Def:            core.Agent(core.AgentDefault),
			Provider:       prov,
			Model:          "stub-model",
			ToolNames:      []string{"echo"},
			Sink:           core.FuncSink(mapper.handle),
		})

		out, err := loop.Run(ctx, []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, core.ReasonEndTurn, out.Reason)
		assert.Equal(t, 1, echo.callCount())
		assert.Contains(t, names, "chat:tool")
		assert.Contains(t, names, "chat:tool-result")
		assert.Contains(t, names, "chat:stream")
	})

	t.Run("未暴露工具被护栏拒绝且不执行", func(t *testing.T) {
		svc, msgRepo := newChatOpsService(t)
		ctx := context.Background()
		ses, _ := seedSession(t, svc, msgRepo, ctx)

		echo := &stubTool{name: "echo", risk: tool.RiskReadOnly}
		require.NoError(t, svc.tools.Registry().Register(echo))

		prov := &stubProvider{turns: []llm.ChatResponse{
			{ToolCalls: []llm.NormalizedToolCall{{ID: "1", Name: "echo", Arguments: json.RawMessage(`{}`)}}, StopReason: "tool_use"},
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "adapted"}, StopReason: "end_turn"},
		}}

		var mu sync.Mutex
		var refused []bool
		svc.bus.Subscribe(event.MatchPrefix("chat:tool-result"), func(_ string, payload any) {
			mu.Lock()
			defer mu.Unlock()
			if m, ok := payload.(map[string]any); ok {
				if v, ok := m["refused"].(bool); ok {
					refused = append(refused, v)
				}
			}
		})

		mapper := newCoreEventMapper(svc, ctx, &domain.ChatSessionDO{ID: ses.ID}, "RUN_1", "MSG_1", nil)
		loop := svc.newCoreLoop(coreLoopSpec{
			Ses:            &domain.ChatSessionDO{ID: ses.ID},
			RunID:          "RUN_1",
			AssistantMsgID: "MSG_1",
			Def:            core.Agent(core.AgentDefault),
			Provider:       prov,
			Model:          "stub-model",
			ToolNames:      []string{"other"}, // echo 未暴露
			Sink:           core.FuncSink(mapper.handle),
		})

		out, err := loop.Run(ctx, []*llm.Message{llm.UserMessage("hi")})
		require.NoError(t, err)

		assert.Equal(t, 0, echo.callCount(), "未暴露的工具绝不能被执行")
		assert.Equal(t, core.ReasonEndTurn, out.Reason, "拒绝不是故障：模型可改道后正常收尾")
		require.Len(t, refused, 1)
		assert.True(t, refused[0], "拒绝回执必须带 refused 标记，前端展示「已拒绝」而非错误")
	})
}

// ===== 定义文件 =====

// TestDefinitionFileParsing 单文件解析：命令模板、子智能体映射、frontmatter 边界与拒绝规则。
func TestDefinitionFileParsing(t *testing.T) {
	t.Run("命令文件", func(t *testing.T) {
		ok, valid := parseCommandFile([]byte("---\ndescription: 审查改动\nargument-hint: \"[范围]\"\n---\n请审查 $ARGUMENTS 的改动"), "review")
		require.True(t, valid)
		assert.Equal(t, "review", ok.Name)
		assert.Equal(t, "审查改动", ok.Desc)
		assert.Equal(t, "[范围]", ok.Args)
		assert.Equal(t, "custom", ok.Group)
		assert.True(t, ok.ClientOnly)
		assert.Contains(t, ok.Prompt, "$ARGUMENTS")
		// 来源由加载器按目录打标（parseCommandFile 只管单个文件的内容）
		assert.Empty(t, ok.Source)

		// 空正文：没有可发送的提示词，跳过而不是产出一个空命令
		if _, v := parseCommandFile([]byte("---\ndescription: 空的\n---\n\n"), "empty"); v {
			t.Fatal("empty prompt must be rejected")
		}
		// 未接入执行链的字段不影响解析（只告警），命令仍然可用
		if _, v := parseCommandFile([]byte("---\nmodel: gpt-4o\nallowed-tools: exec\n---\nbody"), "with-extra"); !v {
			t.Fatal("unsupported fields must not drop the command")
		}
	})

	t.Run("子智能体文件", func(t *testing.T) {
		builtin := map[string]struct{}{"default": {}, "explore": {}}

		def, ok := parseAgentFile([]byte(
			"---\nname: reviewer\ndescription: 代码审查员\ntools: file_read, file_grep\ndisallowedTools: file_write\nmaxTurns: 8\n---\n你是审查员。",
		), "reviewer.md", builtin)
		require.True(t, ok)
		assert.Equal(t, "reviewer", def.Name)
		assert.Equal(t, []string{"file_read", "file_grep"}, def.Tools.Allow)
		assert.Equal(t, []string{"file_write"}, def.Tools.Deny)
		assert.Equal(t, 8, def.Budget.MaxTurns)
		assert.False(t, def.Memory.Enabled, "子智能体不写长期记忆")
		assert.Contains(t, def.Persona, "你是审查员")

		// tools: * → 不设白名单（等价「全部工具」）
		all, ok := parseAgentFile([]byte("---\nname: any\ndescription: d\ntools: \"*\"\n---\nbody"), "any.md", builtin)
		require.True(t, ok)
		assert.Nil(t, all.Tools.Allow)

		// 缺 description / 名非法 / 撞内置名 → 拒绝
		for _, in := range []string{
			"---\nname: no-desc\n---\nbody",
			"---\nname: Bad_Name\ndescription: d\n---\nbody",
			"---\nname: explore\ndescription: 想覆盖内置\n---\nbody",
		} {
			if _, v := parseAgentFile([]byte(in), "x.md", builtin); v {
				t.Fatalf("must be rejected: %s", in)
			}
		}
	})

	t.Run("frontmatter 边界", func(t *testing.T) {
		// 无 frontmatter：整份内容即正文
		plain := "just a prompt"
		fm := splitFrontMatter(plain)
		assert.Equal(t, plain, fm.body)
		assert.Empty(t, fm.get("description"))

		// 列表 / 整数 / 布尔解析，缺省值可指定
		typed := splitFrontMatter("---\ntools: [file_read, file_grep]\nmaxTurns: 12\ninjectAgentsMd: false\n---\nb")
		assert.Equal(t, []string{"file_read", "file_grep"}, typed.list("tools"))
		assert.Equal(t, 12, typed.intValue("maxTurns", 0))
		assert.False(t, typed.boolean("injectAgentsMd", true))
		assert.Equal(t, 7, typed.intValue("missing", 7))

		// 未闭合的 --- 块不是 frontmatter，整体当正文处理（否则会把正文吃掉）
		broken := "---\ndescription: broken"
		bfm := splitFrontMatter(broken)
		assert.Equal(t, broken, bfm.body)
		assert.Empty(t, bfm.get("description"))
	})
}

// TestLoadDefinitionFiles 目录级加载：坏文件跳过、好文件进入注册表（不因一个文件写坏而整体失败）。
func TestLoadDefinitionFiles(t *testing.T) {
	home := t.TempDir()
	cmdDir := filepath.Join(home, "commands")
	agentDir := filepath.Join(home, "agents")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "review.md"),
		[]byte("---\ndescription: 审查\n---\n审查 $1 与 $2"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "Bad Name.md"), []byte("---\n---\nbody"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "notes.txt"), []byte("ignored"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "helper.md"),
		[]byte("---\nname: helper\ndescription: 助手\n---\nbody"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "default.md"),
		[]byte("---\nname: default\ndescription: 撞内置\n---\nbody"), 0o644))

	cmds := LoadCommandFiles(home)
	require.Len(t, cmds, 1, "只有合法命名的 .md 命令文件应被加载")
	assert.Equal(t, "review", cmds[0].Name)
	assert.Equal(t, domain.CommandSourceFile, cmds[0].Source)

	agents := LoadAgentFiles(home)
	require.Len(t, agents, 1, "撞内置名的定义文件应被拒绝")
	assert.Equal(t, "helper", agents[0].Name)
	assert.IsType(t, core.Definition{}, agents[0])

	// 工作区级命令：<ws>/.workbaby/commands，来源标记与用户级区分
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, runtime.SandboxDirName, "commands"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, runtime.SandboxDirName, "commands", "deploy.md"),
		[]byte("---\ndescription: 本项目发布流程\n---\n按项目规范发布"), 0o644))
	wsCmds := LoadWorkspaceCommandFiles(ws)
	require.Len(t, wsCmds, 1)
	assert.Equal(t, "deploy", wsCmds[0].Name)
	assert.Equal(t, domain.CommandSourceWorkspace, wsCmds[0].Source)

	// 目录不存在：返回空而不是报错（首次启动的正常状态）
	assert.Nil(t, LoadCommandFiles(filepath.Join(home, "nope")))
	assert.Nil(t, LoadAgentFiles(filepath.Join(home, "nope")))
	assert.Nil(t, LoadCommandFiles(""))
	// 未绑定工作区：工作区级命令不加载（与工作区技能同一约定）
	assert.Nil(t, LoadWorkspaceCommandFiles(""))
	assert.Equal(t, "", WorkspaceCommandDir(""))
}

// TestAgentModelThinking 模型与推理强度的映射和校验。
//
// 两个入口同一套规则：定义文件（parseAgentFile）与设置页（validateProfileReq）。
// 规则的核心是「推理强度脱离模型就没有意义」——只配强度不配模型会让用户当次的
// 推理档位选择静默失效，所以两处都直接拒绝而不是存下来。
func TestAgentModelThinking(t *testing.T) {
	builtin := map[string]struct{}{"default": {}}

	t.Run("定义文件入口", func(t *testing.T) {
		// model + thoughtLevel：两者都生效
		def, ok := parseAgentFile([]byte(
			"---\nname: heavy\ndescription: 重活\ntools: \"*\"\nmodel: gpt-5\nthoughtLevel: HIGH\n---\nbody",
		), "heavy.md", builtin)
		require.True(t, ok)
		assert.Equal(t, "gpt-5", def.Model)
		assert.Equal(t, "high", def.Thinking, "推理强度应归一为小写")
		assert.Equal(t, "gpt-5", def.EffectiveModel("session-model"))
		// 未指定模型：跟随会话
		assert.Equal(t, "session-model", core.Definition{}.EffectiveModel("session-model"))

		// thoughtLevel 未配 model：不生效（否则会让用户当次的推理档位选择失效）
		only, ok := parseAgentFile([]byte("---\nname: level-only\ndescription: d\nthoughtLevel: high\n---\nbody"), "l.md", builtin)
		require.True(t, ok)
		assert.Empty(t, only.Thinking)

		// 非法档位：丢弃而不是写入
		bad, ok := parseAgentFile([]byte("---\nname: bad\ndescription: d\nmodel: m\nthoughtLevel: ultra\n---\nbody"), "b.md", builtin)
		require.True(t, ok)
		assert.Empty(t, bad.Thinking)

		// inherit 与不写等价
		inherit, ok := parseAgentFile([]byte("---\nname: inh\ndescription: d\nmodel: inherit\n---\nbody"), "i.md", builtin)
		require.True(t, ok)
		assert.Empty(t, inherit.Model)
	})

	t.Run("设置页入口", func(t *testing.T) {
		base := domain.AgentProfileREQ{Name: "reviewer", Description: "审查"}

		_, err := validateProfileReq(&domain.AgentProfileREQ{Name: base.Name, Description: base.Description, Thinking: "high"})
		require.Error(t, err, "只给推理强度不给模型必须被拒")

		_, err = validateProfileReq(&domain.AgentProfileREQ{Name: base.Name, Description: base.Description, Model: "gpt-5", Thinking: "ultra"})
		require.Error(t, err, "非法档位必须被拒")

		row, err := validateProfileReq(&domain.AgentProfileREQ{
			Name: base.Name, Description: base.Description, Model: "gpt-5", Thinking: "HIGH",
		})
		require.NoError(t, err)
		assert.Equal(t, "gpt-5", row.Model)
		assert.Equal(t, "high", row.Thinking)

		def := profileToDefinition(row)
		assert.Equal(t, "gpt-5", def.Model)
		assert.Equal(t, "high", def.Thinking)
	})
}

// ===== 自定义档案 =====

// TestAgentProfileLifecycle 校验 upsert → 注册表同步 → 启停 → 删除全链路。
func TestAgentProfileLifecycle(t *testing.T) {
	gdb := newChatOpsTestDB(t)
	svc := NewAgentProfileService(repo.NewAgentProfileRepo(gdb))
	ctx := t.Context()

	enabled := true
	created, err := svc.Upsert(ctx, &domain.AgentProfileREQ{
		Name:         "translator",
		Description:  "中英翻译",
		SystemPrompt: "你是翻译助手，只输出译文。",
		ToolsAllow:   []string{"webfetch"},
		MaxTurns:     6,
		Enabled:      &enabled,
	})
	require.NoError(t, err)
	require.Equal(t, "translator", created.Name)
	require.Equal(t, []string{"webfetch"}, created.ToolsAllow)

	// 注册表已同步：core.Agent 命中自定义定义
	def := core.Agent("translator")
	require.Equal(t, "translator", def.Name)
	require.Equal(t, "你是翻译助手，只输出译文。", def.Persona)
	require.Equal(t, 6, def.Budget.MaxTurns)
	found := false
	for _, d := range core.AllAgents() {
		if d.Name == "translator" {
			found = true
		}
	}
	require.True(t, found)

	// 内置保留名拒绝（内置已收敛为 default / explore）
	_, err = svc.Upsert(ctx, &domain.AgentProfileREQ{Name: core.AgentExplore})
	require.Error(t, err)

	// 非法名拒绝
	_, err = svc.Upsert(ctx, &domain.AgentProfileREQ{Name: "Bad Name!"})
	require.Error(t, err)

	// 停用后从注册表摘除
	require.NoError(t, svc.SetEnabled(ctx, "translator", false))
	require.Equal(t, "default", core.Agent("translator").Name)

	// 重新启用即回归注册表
	require.NoError(t, svc.SetEnabled(ctx, "translator", true))
	require.Equal(t, "translator", core.Agent("translator").Name)

	// 删除后回退 default
	require.NoError(t, svc.Delete(ctx, "translator"))
	require.Equal(t, "default", core.Agent("translator").Name)
	_, err = svc.List(ctx)
	require.NoError(t, err)
}
