// 服务层链路：会话索引、一轮对话与用量汇总、停止、目录扫描（技能/插件）、配置写入（模型/MCP）、跨工作区并行。
// 坏了的表现：界面上什么都看不到、对话发不出去、改了配置助手行为不变。
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/config"
	"WorkBaby/backend/db"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pi/piptest"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/runtime/runtimetest"
	"WorkBaby/backend/skill"
)

// testEnv 是一套隔离的装配，供全部子测试共用一份昂贵 setup。
type testEnv struct {
	env       *Env
	container *Container
	home      string
	events    *eventSink
}

// eventSink 收集事件，供断言顺序。
type eventSink struct {
	mu   sync.Mutex
	list []domain.Envelope
}

func (s *eventSink) collect(_ string, env domain.Envelope) {
	s.mu.Lock()
	s.list = append(s.list, env)
	s.mu.Unlock()
}

// names 返回事件名序列。
func (s *eventSink) names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.list))
	for _, e := range s.list {
		out = append(out, e.Event)
	}
	return out
}

// payloadOf 返回第一条指定事件的名字与载荷；没找到就 ok=false。
func (s *eventSink) payloadOf(event string) (map[string]any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.list {
		if e.Event == event {
			raw, _ := json.Marshal(e.Data)
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			return m, true
		}
	}
	return nil, false
}

func (s *eventSink) reset() {
	s.mu.Lock()
	s.list = nil
	s.mu.Unlock()
}

func (s *eventSink) has(event string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.list {
		if e.Event == event {
			return true
		}
	}
	return false
}

// bootService 起一套完整服务（真 DB + 假 PI 进程 + 隔离的 agent 目录）。
func bootService(t *testing.T) *testEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)
	// agent 目录必须隔离：测试绝不能动用户真实的 ~/.pi/agent。
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "agent"))
	t.Setenv("WORKBABY_PI_EXE", piptest.Build(t))
	runtimetest.SeedMarkers(t, home)

	paths, err := runtime.Resolve()
	if err != nil {
		t.Fatalf("解析路径失败: %v", err)
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	cfg.Workspace = t.TempDir()
	gdb, err := db.Open(paths.DBPath)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	r := repo.New(gdb)
	t.Cleanup(func() { _ = r.Close() })

	sink := &eventSink{}
	emitter := NewEmitter()
	emitter.SetSink(sink.collect)

	env := &Env{
		Repo: r, Paths: paths, Cfg: cfg,
		Emitter: emitter, Skills: skill.New(), PiEnv: pienv.New(paths),
	}
	container, err := New(env)
	if err != nil {
		t.Fatalf("装配服务失败: %v", err)
	}
	if err := container.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap 失败: %v", err)
	}
	t.Cleanup(func() { env.PiSupervisor().StopAll() })
	return &testEnv{env: env, container: container, home: home, events: sink}
}

// waitIdle 等到会话不再运行。
func (te *testEnv) waitIdle(t *testing.T, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !te.container.Chat.IsRunning(sessionID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待会话空闲超时")
}

// waitAbortMarker 等假 PI 把「收到 abort」写进会话文件。
// abort 是后台命令，界面广播 stopped 时它可能还在路上，所以只能轮询。
func waitAbortMarker(t *testing.T, file string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := pi.LoadSession(file); err == nil {
			for _, e := range data.Entries {
				if e.Type == "custom" && e.CustomType == "aborted" {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("停止没送到内核：界面说了已停止，助手其实还在跑")
}

// writeSession 手写一份会话文件：会话索引的断言都基于它。
func (te *testEnv) writeSession(t *testing.T, id, cwd, userText string, tokens int) string {
	t.Helper()
	dir := filepath.Join(te.env.Paths.SessionsDir, "--"+strings.ReplaceAll(cwd, ":", "-"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "20260101_"+id+".jsonl")
	lines := []map[string]any{
		{"type": "session", "version": 3, "id": id, "timestamp": "2026-01-01T00:00:00Z", "cwd": cwd},
		{"type": "message", "id": "sys", "parentId": nil, "timestamp": "2026-01-01T00:00:01Z",
			"message": map[string]any{
				"role": "system", "content": "",
				"toolsAdded": []map[string]any{
					{"name": "read", "description": "Read a file"},
					{"name": "custom_tool", "description": "From an extension"},
				},
			}},
		{"type": "message", "id": "u1", "parentId": "sys", "timestamp": "2026-01-01T00:00:02Z",
			"message": map[string]any{"role": "user", "content": userText, "timestamp": 1767225602000}},
		{"type": "message", "id": "a1", "parentId": "u1", "timestamp": "2026-01-01T00:00:03Z",
			"message": map[string]any{
				"role": "assistant", "provider": "p", "model": "m",
				// 内容是块数组：思考 → 正文 → 工具调用。顺序就是界面该渲染的顺序。
				"content": []map[string]any{
					{"type": "thinking", "thinking": "想了想"},
					{"type": "text", "text": "答复"},
					{"type": "toolCall", "id": "tc1", "name": "read", "arguments": map[string]any{"path": "a.txt"}},
				},
				"stopReason": "stop", "timestamp": 1767225603000,
				"usage": map[string]any{
					"input": 100, "output": 20, "cacheRead": 30, "cacheWrite": 10, "totalTokens": tokens,
					"cost": map[string]any{"total": 0.02},
				},
			}},
		{"type": "message", "id": "t1", "parentId": "a1", "timestamp": "2026-01-01T00:00:03Z",
			"message": map[string]any{
				"role": "toolResult", "toolCallId": "tc1", "toolName": "read", "content": "文件内容",
			}},
		{"type": "session_info", "id": "n1", "parentId": "a1", "timestamp": "2026-01-01T00:00:04Z", "name": "手写的会话"},
	}
	var b strings.Builder
	for _, line := range lines {
		raw, _ := json.Marshal(line)
		b.Write(raw)
		b.WriteString("\n")
	}
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

// TestServiceChain 是 service 包唯一的测试入口。五条链各自独立命名，失败时一个名字即定位一件事。
// 子测试按数据依赖顺序排列：后一条用前一条落下的磁盘事实（如真实会话的 usage）。
func TestServiceChain(t *testing.T) {
	te := bootService(t)

	// 会话索引与工具清单是同一条读路径（都来自 PI 的会话文件），删除也在这条链上收尾。
	t.Run("会话索引：列表、投影、工具清单与删除都来自 PI 的会话文件", func(t *testing.T) {
		cwd := t.TempDir()
		file := te.writeSession(t, "SESS_ONE", cwd, "帮我整理一下这份表格", 120)
		defer os.Remove(file)

		list, err := te.container.Sessions.List("")
		if err != nil {
			t.Fatalf("列会话失败: %v", err)
		}
		var hit domain.SessionVO
		for _, s := range list {
			if s.ID == "SESS_ONE" {
				hit = s
			}
		}
		// 标题取 session_info、工作目录来自会话头：界面读到的就是 PI 记的。
		if hit.Title != "手写的会话" || hit.Workspace != cwd || hit.TotalTokens != 120 {
			t.Fatalf("会话元信息不对: %+v", hit)
		}

		detail, err := te.container.Sessions.Detail("SESS_ONE")
		if err != nil {
			t.Fatalf("取详情失败: %v", err)
		}
		if len(detail.Messages) != 3 {
			t.Fatalf("投影出 %d 条消息，期望 3 条（用户 + 助手 + 工具结果）", len(detail.Messages))
		}
		if detail.Messages[1].Content != "答复" || detail.Messages[1].Usage == nil {
			t.Fatalf("助手消息投影不对: %+v", detail.Messages[1])
		}

		// 内容块顺序：界面按它就地渲染。工具卡被整体搬到正文前面 = 顺序丢了。
		blocks := detail.Messages[1].Blocks
		if len(blocks) != 3 || blocks[0].Kind != domain.BlockThinking ||
			blocks[1].Kind != domain.BlockText || blocks[2].Kind != domain.BlockTool {
			t.Fatalf("内容块顺序不对: %+v", blocks)
		}
		if blocks[2].ToolCallID != "tc1" || blocks[2].Args["path"] != "a.txt" {
			t.Fatalf("工具块参数丢了: %+v", blocks[2])
		}
		if detail.Messages[2].Role != domain.RoleTool || detail.Messages[2].ToolCallID != "tc1" {
			t.Fatalf("工具结果投影不对: %+v", detail.Messages[2])
		}

		// 用量口径：提示词总量 = 未命中 + 命中 + 写入；命中量不与 input 混在一起。
		usage := detail.Messages[1].Usage
		if usage.Cached != 30 || usage.CacheWrite != 10 || usage.Context != 140 {
			t.Fatalf("用量口径不对: %+v", usage)
		}

		// 工具清单来自会话里的工具声明：这才是 PI 真能调到的工具。
		catalog := te.container.Tools.Catalog()
		if catalog.Snapshot != "session" || !hasTool(catalog.Tools, "custom_tool") {
			t.Fatalf("工具清单没读到会话声明: snapshot=%q tools=%v", catalog.Snapshot, catalog.Tools)
		}

		// 删除 = 删 PI 的会话文件，不是只删列表里的一行。
		if err := te.container.Sessions.Delete("SESS_ONE"); err != nil {
			t.Fatalf("删除会话失败: %v", err)
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("删除会话应当删掉会话文件")
		}
	})

	t.Run("一轮对话：新会话现场创建、done 最后、落盘后能算出用量", func(t *testing.T) {
		te.events.reset()
		resp, err := te.container.Chat.Send(domain.SendMessageREQ{
			Workspace: te.env.Cfg.Workspace,
			Content:   "你好",
		})
		if err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		if resp.SessionID == "" {
			t.Fatal("新对话必须返回会话编号")
		}
		te.waitIdle(t, resp.SessionID)

		names := te.events.names()
		// done 必须是最后一个「结束」信号：重试与压缩都挂在 agent_end 之后。
		// （流式增量与正文拼接的还原在 api 的 SSE 全链里断言，这里只管收尾次序。）
		if names[len(names)-1] != domain.EventChatDone {
			t.Fatalf("最后一个事件应当是 chat:done，实际 %q", names[len(names)-1])
		}
		// 自动重试期间界面必须收到 chat:retry，否则会以为卡死。
		if !te.events.has(domain.EventChatRetry) {
			t.Fatalf("没有 chat:retry: %v", names)
		}

		// 假 PI 也落了盘：这条会话应当出现在列表里（会话索引与对话是两条独立的读路径）。
		list, err := te.container.Sessions.List(te.env.Cfg.Workspace)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) == 0 || list[0].ID != resp.SessionID {
			t.Fatalf("新会话没有出现在列表里: %+v", list)
		}

		// 用量从这条真会话的 usage 汇总：补齐 7 天、按模型归因、总量非零。
		stats, err := te.container.Stats.Compute("7d")
		if err != nil {
			t.Fatalf("统计失败: %v", err)
		}
		if len(stats.Daily) != 7 || stats.Totals.Total == 0 || len(stats.Models) == 0 {
			t.Fatalf("统计不对: daily=%d total=%d models=%d", len(stats.Daily), stats.Totals.Total, len(stats.Models))
		}
	})

	// 停止与空闲排队是同一条队列链路的两面：停止先取回队列再中止；
	// 空闲时收到的 follow_up 等价于一轮新发送（不记账的话翻译器没有 runState，整轮回应消失）。
	t.Run("停止与空闲排队：先取回队列再中止，follow_up 按完整一轮记账", func(t *testing.T) {
		resp, err := te.container.Chat.Send(domain.SendMessageREQ{
			Workspace: te.env.Cfg.Workspace, Content: "跑一轮",
		})
		if err != nil {
			t.Fatal(err)
		}
		te.events.reset()
		if err := te.container.Chat.Stop(resp.SessionID); err != nil {
			t.Fatalf("停止失败: %v", err)
		}
		te.waitIdle(t, resp.SessionID)
		if !te.events.has(domain.EventChatUser) {
			t.Fatal("排队的那句话没取回来：用户会以为它凭空消失")
		}
		if !te.events.has(domain.EventChatStopped) {
			t.Fatal("停止必须立刻广播 chat:stopped")
		}
		// 广播了 stopped 不等于内核真停了：abort 是后台命令，得看内核自己落的标记。
		vo, err := te.container.Sessions.Get(resp.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		waitAbortMarker(t, vo.File)

		// 空闲 follow_up 回归：等价于完整一轮，start / delta / done / 回显一个都不能少。
		te.events.reset()
		if err := te.container.Chat.FollowUp(domain.SteerREQ{
			SessionID: resp.SessionID, Content: "空闲时排的队",
		}); err != nil {
			t.Fatal(err)
		}
		te.waitIdle(t, resp.SessionID)
		for _, ev := range []string{domain.EventChatStart, domain.EventChatDelta, domain.EventChatDone, domain.EventChatUser} {
			if !te.events.has(ev) {
				t.Fatalf("空闲排队那一轮缺 %s: %v", ev, te.events.names())
			}
		}

		// 内核与扩展按终端习惯给文字上色，GUI 没有终端语义：
		// 不剥就是消息流里的 [39m[2mCache 0.0%[22;39m 这种乱码。
		te.events.reset()
		resp2, err := te.container.Chat.Send(domain.SendMessageREQ{
			Workspace: te.env.Cfg.Workspace, Content: "触发状态条",
		})
		if err != nil {
			t.Fatal(err)
		}
		te.waitIdle(t, resp2.SessionID)
		status, ok := te.events.payloadOf(domain.EventChatStatus)
		if !ok {
			t.Fatalf("扩展的状态条事件没到: %v", te.events.names())
		}
		if text, _ := status["text"].(string); text != "Indexing workspace..." {
			t.Fatalf("状态条文本没剥成纯文本（实际 %q）", text)
		}
	})

	// 配置与目录扫描是同一条回路：界面上改的东西都要落到 PI 的目录与文件里，
	// 再从那里读回来。坏了的表现是「界面显示改好了，助手行为没变」。
	t.Run("配置落盘与回读：技能三区启停、插件登记项、models.json 与 mcp.json", func(t *testing.T) {
		skillsDir := te.env.PiEnv.SkillsDir()
		mkSkill(t, filepath.Join(skillsDir, "demo-skill"), "demo-skill", "演示")
		mkSkill(t, filepath.Join(te.env.PiEnv.DisabledSkillsDir(), "old-skill"), "old-skill", "停用的")

		// 一个装了技能的包：settings.json 里登记，目录在 agent/npm/node_modules 下。
		pkgDir := filepath.Join(te.env.PiEnv.AgentDir(), "npm", "node_modules", "pi-demo-pkg")
		mkSkill(t, filepath.Join(pkgDir, "skills", "pkg-skill"), "pkg-skill", "包里的")
		extDir := te.env.PiEnv.ExtensionsDir()
		if err := os.MkdirAll(extDir, 0o755); err != nil {
			t.Fatal(err)
		}
		ext := filepath.Join(extDir, "extra.ts")
		if err := os.WriteFile(ext, []byte("export default () => {}"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeSettings(t, te.env, []string{"npm:pi-demo-pkg"}, []string{ext})

		te.container.Skills.Reload("")
		list := te.container.Skills.List()
		if !hasSkill(list, "demo-skill", domain.SkillSourceGlobal) {
			t.Fatalf("用户技能没扫到: %+v", list)
		}
		if !hasSkill(list, "old-skill", domain.SkillSourceGlobal) {
			t.Fatalf("停用区的技能也要列出来: %+v", list)
		}
		if !hasSkill(list, "pkg-skill", domain.SkillSourcePackage) {
			t.Fatalf("包自带技能没扫到: %+v", list)
		}

		if err := te.container.Skills.Toggle("demo-skill@global", false); err != nil {
			t.Fatalf("停用技能失败: %v", err)
		}
		if dirExists(filepath.Join(skillsDir, "demo-skill")) {
			t.Fatal("停用后目录应当移出 PI 的扫描范围")
		}
		if !dirExists(filepath.Join(te.env.PiEnv.DisabledSkillsDir(), "demo-skill")) {
			t.Fatal("停用的技能应当落在 skills.disabled 里")
		}
		if err := te.container.Skills.Toggle("pkg-skill@package", false); err == nil {
			t.Fatal("包自带技能不该被界面搬动")
		}

		exts, err := te.container.Extensions.List()
		if err != nil {
			t.Fatalf("列插件失败: %v", err)
		}
		if len(exts.Packages) != 1 || exts.Packages[0].Name != "pi-demo-pkg" {
			t.Fatalf("包清单不对: %+v", exts.Packages)
		}
		if exts.Packages[0].Missing {
			t.Fatalf("已落地的包不该标 missing: %+v", exts.Packages[0])
		}
		// 版本与种类直接决定插件页能不能显示「可更新」：认错种类就不会走到 pi update。
		if exts.Packages[0].Kind != domain.PackageKindNpm {
			t.Fatalf("npm 包种类识别错了: %+v", exts.Packages[0])
		}
		if len(exts.Entries) != 1 || !exts.Entries[0].Enabled {
			t.Fatalf("登记的扩展条目不对: %+v", exts.Entries)
		}
		// 停用扩展 = 改登记表（文件不搬走），并且要把登记项从 settings.json 摘掉。
		if err := te.container.Extensions.Toggle(ext, false); err != nil {
			t.Fatalf("停用扩展失败: %v", err)
		}
		after, _ := te.env.PiEnv.ReadSettings()
		if len(after.Extensions) != 0 {
			t.Fatalf("停用后登记项应当被摘掉: %v", after.Extensions)
		}

		// 服务商：密钥明文只落在 models.json 里（PI 就是这么读的），列表只给掩码。
		// 校验顺序与界面字段顺序一致：名字 → 地址 → 模型，一次只说一件事。
		if _, err := te.container.Providers.Upsert(domain.UpsertProviderREQ{
			API: domain.APIOpenAICompletions, BaseURL: "http://127.0.0.1:1234/v1",
			Models: []domain.ModelREQ{{ID: "m1"}},
		}); err == nil || !strings.Contains(err.Error(), domain.ErrProviderName.Message) {
			t.Fatalf("空名字应当报 3109，而不是拿模型 ID 顶包: %v", err)
		}
		if _, err := te.container.Providers.Upsert(domain.UpsertProviderREQ{
			Name: "没地址", API: domain.APIOpenAICompletions,
			Models: []domain.ModelREQ{{ID: "m1"}},
		}); err == nil || !strings.Contains(err.Error(), domain.ErrProviderBaseURL.Message) {
			t.Fatalf("空地址应当报 3113: %v", err)
		}
		if _, err := te.container.Providers.Upsert(domain.UpsertProviderREQ{
			Name: "空模型行", API: domain.APIOpenAICompletions, BaseURL: "http://127.0.0.1:1234/v1",
			Models: []domain.ModelREQ{{ID: ""}},
		}); err == nil || !strings.Contains(err.Error(), domain.ErrProviderModelID.Message) {
			t.Fatalf("空白模型 ID 应当报 3111（而不是静默丢掉）: %v", err)
		}

		vo, err := te.container.Providers.Upsert(domain.UpsertProviderREQ{
			Name: "本地网关", API: domain.APIOpenAICompletions,
			BaseURL: "http://127.0.0.1:1234/v1", APIKey: "sk-secret-1234567890",
			Models: []domain.ModelREQ{{ID: "m1", Name: "模型一", ContextWindow: 128000, MaxTokens: 8192, Vision: true}},
		})
		if err != nil || vo.ID == "" {
			t.Fatalf("写服务商失败: %v vo=%+v", err, vo)
		}
		plist, err := te.container.Providers.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(plist.Providers) != 1 || strings.Contains(plist.Providers[0].KeyHint, "secret-1234") {
			t.Fatalf("服务商清单没落盘或掩码泄露: %+v", plist.Providers)
		}
		if err := te.container.Providers.SetDefault(domain.SetDefaultREQ{Provider: vo.ID, Model: "m1"}); err != nil {
			t.Fatal(err)
		}
		if p, m := te.container.Providers.Defaults(); p != vo.ID || m != "m1" {
			t.Fatalf("默认模型没写进 settings.json: %s/%s", p, m)
		}

		// MCP：坏 JSON 一个字符都不写。
		if err := te.container.MCP.Save(domain.SaveMCPREQ{Scope: "global", Raw: `{"mcpServers":{`}); err == nil {
			t.Fatal("坏 JSON 必须被拒")
		}
		rawMCP, _ := te.env.PiEnv.MCPRaw(te.env.PiEnv.MCPPath())
		if strings.Contains(string(rawMCP), "mcpServers") {
			t.Fatalf("坏配置被写进去了: %s", rawMCP)
		}
		if err := te.container.MCP.Save(domain.SaveMCPREQ{
			Scope: "global", Raw: `{"mcpServers":{"demo":{"command":"node","args":["x.js"]}}}`,
		}); err != nil {
			t.Fatalf("保存 MCP 失败: %v", err)
		}
		mcpList, err := te.container.MCP.List("")
		if err != nil {
			t.Fatal(err)
		}
		if len(mcpList.Servers) == 0 || mcpList.Servers[0].Name != "demo" || mcpList.Servers[0].Scope != "global" {
			t.Fatalf("服务没读出来: %+v", mcpList.Servers)
		}

		// 删掉服务商要连带清掉默认模型，否则下次新对话指向一个不存在的服务。
		if err := te.container.Providers.Delete(vo.ID); err != nil {
			t.Fatal(err)
		}
		if p, _ := te.container.Providers.Defaults(); p != "" {
			t.Fatal("删掉服务商后默认模型应一起清掉")
		}
	})

	t.Run("不同工作区可并行：各自一个进程，不串会话", func(t *testing.T) {
		var wg sync.WaitGroup
		ids := make([]string, 2)
		errs := make([]error, 2)
		workspaces := make([]string, 2)
		for i := 0; i < 2; i++ {
			workspaces[i] = t.TempDir()
			wg.Add(1)
			go func(idx int, workspace string) {
				defer wg.Done()
				resp, err := te.container.Chat.Send(domain.SendMessageREQ{
					Workspace: workspace, Content: "并发",
				})
				if err != nil {
					errs[idx] = err
					return
				}
				ids[idx] = resp.SessionID
			}(i, workspaces[i])
		}
		wg.Wait()
		// 子进程的 cwd 就在这些临时目录里：先收进程，否则目录删不掉。
		for _, ws := range workspaces {
			te.env.PiSupervisor().Stop(ws)
		}
		for i, err := range errs {
			if err != nil {
				t.Fatalf("第 %d 个工作区发送失败: %v", i, err)
			}
		}
		if ids[0] == ids[1] || ids[0] == "" {
			t.Fatalf("两个工作区拿到了同一个会话: %v", ids)
		}
	})
}

// mkSkill 造一个技能目录。
func mkSkill(t *testing.T, dir, name, desc string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n按步骤做。\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSettings 写一份 settings.json（packages + extensions）。
func writeSettings(t *testing.T, env *Env, packages, extensions []string) {
	t.Helper()
	file, err := env.PiEnv.ReadSettings()
	if err != nil || file == nil {
		file = &pienv.SettingsFile{}
	}
	if packages != nil {
		raw, _ := json.Marshal(packages)
		body := map[string]json.RawMessage{}
		if existing, err := env.PiEnv.ReadSettingsMap(); err == nil {
			body = existing
		}
		body["packages"] = raw
		encoded, _ := json.MarshalIndent(body, "", "  ")
		if err := os.MkdirAll(env.PiEnv.AgentDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(env.PiEnv.SettingsPath(), append(encoded, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if extensions != nil {
		file.Extensions = extensions
		if err := env.PiEnv.WriteSettings(file); err != nil {
			t.Fatal(err)
		}
	}
}

func hasTool(list []domain.ToolVO, name string) bool {
	for _, t := range list {
		if t.Name == name {
			return true
		}
	}
	return false
}

func hasSkill(list []domain.SkillVO, name, source string) bool {
	for _, s := range list {
		if s.Name == name && s.Source == source {
			return true
		}
	}
	return false
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
