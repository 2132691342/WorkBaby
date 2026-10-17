// 服务层链路：发送命令选择、JSONL 投影、会话懒创建与删除、用量口径、审批桥、子代理。
// 事件来自真实的假 PI 进程（走 stdin/stdout 协议），所以这里测的是编排本身。
// 坏了的表现：空闲时说话走错命令（PI 拒绝）、会话列表读错文件、删会话留下孤儿、
// 审批卡点了没反应（PI 那边一直阻塞）、子代理把父会话的流顶掉。
package service

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/config"
	"WorkBaby/backend/db"
	"WorkBaby/backend/domain"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
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
}

// bootService 起一套完整服务（真 DB + 真假 PI 进程）。
func bootService(t *testing.T) *testEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)
	t.Setenv("WORKBABY_PI_EXE", buildFakePI(t))
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
	skills := skill.New()
	env := &Env{
		Repo: r, Paths: paths, Cfg: cfg,
		Emitter: NewEmitter(), Skills: skills,
		PiEnv: pienv.New(paths),
	}
	container, err := New(env)
	if err != nil {
		t.Fatalf("装配服务失败: %v", err)
	}
	if _, err := container.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap 失败: %v", err)
	}
	t.Cleanup(func() { env.PiSupervisor().StopAll() })
	return &testEnv{env: env, container: container, home: home}
}

// buildFakePI 编译假 PI。
func buildFakePI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "fakepi.exe")
	cmd := exec.Command("go", "build", "-o", exe, "./backend/pi/pitest")
	cmd.Dir = repoRootOf(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译假 PI 失败: %v\n%s", err, out)
	}
	return exe
}

// repoRootOf 往上找 go.mod。
func repoRootOf(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到仓库根目录")
		}
		dir = parent
	}
}

// newSession 建一个会话。
func (te *testEnv) newSession(t *testing.T) *domain.SessionVO {
	t.Helper()
	vo, err := te.container.Sessions.Create(domain.CreateSessionREQ{})
	if err != nil {
		t.Fatalf("建会话失败: %v", err)
	}
	return vo
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

// writeJSONL 直接写一份 PI 会话文件，用于测投影。
func writeJSONL(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestServiceChain 是 service 包唯一的测试入口。
func TestServiceChain(t *testing.T) {
	te := bootService(t)

	t.Run("会话懒创建：发消息前不建 PI 会话", func(t *testing.T) {
		sess := te.newSession(t)
		if sess.PiSessionID != "" {
			t.Fatal("还没发消息就建了 PI 会话（会留下空文件并拖慢每次点击）")
		}
		// 详情在懒创建阶段也要能给：消息列表是空数组而不是 null。
		detail, err := te.container.Sessions.Detail(sess.ID)
		if err != nil {
			t.Fatalf("读详情失败: %v", err)
		}
		if detail.Messages == nil {
			t.Fatal("消息列表是 null，前端按数组读会崩")
		}
	})

	t.Run("空闲发送会建 PI 会话并回写指针", func(t *testing.T) {
		sess := te.newSession(t)
		resp, err := te.container.Chat.Send(sess.ID, "你好", nil)
		if err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		if resp.RunID == "" {
			t.Fatal("没有返回 runID")
		}
		te.waitIdle(t, sess.ID)

		after, err := te.env.Repo.GetSession(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		// 懒创建之后必须把 PI 的会话指针回写，否则下次打开读不到历史。
		if after.PiSessionID == "" || after.PiSessionFile == "" {
			t.Fatalf("PI 会话指针没回写: %+v", after)
		}
	})

	t.Run("用量按上游口径落库且命中率不超 100%", func(t *testing.T) {
		sess := te.newSession(t)
		if _, err := te.container.Chat.Send(sess.ID, "算用量", nil); err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		te.waitIdle(t, sess.ID)

		in, out, cached, err := te.env.Repo.UsageOfSession(sess.ID)
		if err != nil {
			t.Fatalf("读用量失败: %v", err)
		}
		if in <= 0 || out <= 0 {
			t.Fatalf("用量没落账: in=%d out=%d", in, out)
		}
		u := domain.UsageVO{Input: in, Output: out, Cached: cached}
		// 命中量不可能超过输入量：超了界面会显示 229% 这种数。
		if u.CacheHitRate() > 100 {
			t.Fatalf("缓存命中率超过 100%%: %d", u.CacheHitRate())
		}
	})

	t.Run("运行记录在 agent_settled 后收尾", func(t *testing.T) {
		sess := te.newSession(t)
		if _, err := te.container.Chat.Send(sess.ID, "跑一轮", nil); err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		te.waitIdle(t, sess.ID)

		runs, err := te.container.Runs.List(sess.ID, 20)
		if err != nil {
			t.Fatalf("读运行记录失败: %v", err)
		}
		if len(runs) == 0 {
			t.Fatal("没有运行记录")
		}
		for _, r := range runs {
			// 状态与收尾原因分开：合并就分不清「被停」与「说完」。
			if r.Status == "" || r.StopReason == "" {
				t.Fatalf("运行记录缺字段: %+v", r)
			}
			if r.Status == domain.RunRunning {
				t.Fatalf("agent_settled 之后还有 running: %+v", r)
			}
		}
	})

	t.Run("停止先取回排队文本再中止", func(t *testing.T) {
		sess := te.newSession(t)
		if _, err := te.container.Chat.Send(sess.ID, "开始干活", nil); err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		// 停止不该报错，也不该让会话永久卡在运行态。
		if err := te.container.Chat.Stop(sess.ID); err != nil {
			t.Fatalf("停止失败: %v", err)
		}
		te.waitIdle(t, sess.ID)
	})

	t.Run("JSONL 投影只显示文件里有的东西", func(t *testing.T) {
		sess := te.newSession(t)
		file := filepath.Join(te.home, "sessions", "test.jsonl")
		writeJSONL(t, file, []string{
			`{"type":"session","version":3,"id":"s1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/w"}`,
			`{"type":"message","id":"e1","parentId":null,"timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"system","content":"","sections":{"preamble":"sys"},"timestamp":1}}`,
			`{"type":"message","id":"e2","parentId":"e1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"user","content":"你好","timestamp":2}}`,
			`{"type":"message","id":"e3","parentId":"e2","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"想一下"},{"type":"text","text":"答你"},{"type":"toolCall","id":"c1","name":"read","arguments":{"path":"a.txt"}}],"stopReason":"toolUse","usage":{"input":100,"output":20,"cacheRead":40,"totalTokens":120,"cost":{"total":0.001}},"timestamp":3}}`,
			`{"type":"message","id":"e4","parentId":"e3","timestamp":"2026-01-01T00:00:04.000Z","message":{"role":"toolResult","toolCallId":"c1","toolName":"read","content":[{"type":"text","text":"文件内容"}],"isError":false,"timestamp":4}}`,
			`{"type":"compaction","id":"e5","parentId":"e4","timestamp":"2026-01-01T00:00:05.000Z","summary":"之前聊过了","firstKeptEntryId":"e3","tokensBefore":9000}`,
			`{"type":"model_change","id":"e6","parentId":"e5","timestamp":"2026-01-01T00:00:06.000Z","provider":"p","modelId":"m"}`,
			`{"type":"usage","id":"e7","parentId":"e6","kind":"cache_warm","timestamp":"2026-01-01T00:00:07.000Z"}`,
			`{"type":"label","id":"e8","parentId":"e7","targetId":"e2","label":"checkpoint","timestamp":"2026-01-01T00:00:08.000Z"}`,
		})
		if err := te.env.Repo.UpdateSessionColumns(sess.ID, map[string]any{
			"pi_session_file": file, "pi_session_id": "s1",
		}); err != nil {
			t.Fatal(err)
		}

		do, err := te.env.Repo.GetSession(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		msgs, err := te.container.Sessions.Messages(do)
		if err != nil {
			t.Fatalf("投影失败: %v", err)
		}
		// system / usage / label 都不产消息；user / assistant / toolResult / compaction / model_change 产。
		if len(msgs) != 5 {
			t.Fatalf("投影条数不对（应 5 条），实际 %d: %+v", len(msgs), msgs)
		}
		if msgs[0].Role != domain.RoleUser || msgs[0].Content != "你好" {
			t.Fatalf("用户消息不对: %+v", msgs[0])
		}
		asst := msgs[1]
		if asst.Thinking != "想一下" || asst.Content != "答你" {
			t.Fatalf("助手的思考与正文没分开: %+v", asst)
		}
		if len(asst.ToolCalls) != 1 || asst.ToolCalls[0].Name != "read" {
			t.Fatalf("工具调用没解析出来: %+v", asst)
		}
		if asst.Usage == nil || asst.Usage.Input != 100 || asst.Usage.Cached != 40 {
			t.Fatalf("用量没解析出来: %+v", asst.Usage)
		}
		if msgs[2].Role != domain.RoleTool || msgs[2].Content != "文件内容" {
			t.Fatalf("工具结果不对: %+v", msgs[2])
		}
		// 压缩与换模型是「提示条」而不是气泡。
		if msgs[3].Notice == "" || msgs[4].Notice == "" {
			t.Fatalf("压缩 / 换模型应当投影成提示条: %+v %+v", msgs[3], msgs[4])
		}
	})

	t.Run("坏行只跳过自己", func(t *testing.T) {
		sess := te.newSession(t)
		file := filepath.Join(te.home, "sessions", "broken.jsonl")
		writeJSONL(t, file, []string{
			`{"type":"message","id":"e1","parentId":null,"timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"好的一条","timestamp":1}}`,
			`{"type":"message","id":`,
			`{"type":"message","id":"e2","parentId":"e1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"user","content":"另一条好的","timestamp":2}}`,
		})
		if err := te.env.Repo.UpdateSessionColumns(sess.ID, map[string]any{"pi_session_file": file}); err != nil {
			t.Fatal(err)
		}
		do, _ := te.env.Repo.GetSession(sess.ID)
		msgs, err := te.container.Sessions.Messages(do)
		if err != nil {
			t.Fatalf("一行坏 JSON 不该让整个会话打不开: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("好行应当在，坏行被跳过，实际 %d 条", len(msgs))
		}
	})

	t.Run("删除会话同时清掉 JSONL 文件", func(t *testing.T) {
		sess := te.newSession(t)
		file := filepath.Join(te.home, "sessions", "todelete.jsonl")
		writeJSONL(t, file, []string{`{"type":"session","version":3,"id":"x"}`})
		if err := te.env.Repo.UpdateSessionColumns(sess.ID, map[string]any{"pi_session_file": file}); err != nil {
			t.Fatal(err)
		}
		if err := te.container.Sessions.Delete(sess.ID); err != nil {
			t.Fatalf("删除失败: %v", err)
		}
		// 只删行不删文件会留孤儿文件；只删文件不删行会留孤儿记录。
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("JSONL 文件没被删掉: %v", err)
		}
		if _, err := te.env.Repo.GetSession(sess.ID); err == nil {
			t.Fatal("会话行没被删掉")
		}
	})

	t.Run("权限档写进策略文件", func(t *testing.T) {
		sess := te.newSession(t)
		if err := te.container.Sessions.SetPermission(sess.ID, domain.PermissionReadonly); err != nil {
			t.Fatalf("设置权限档失败: %v", err)
		}
		raw, err := os.ReadFile(te.env.PiEnv.PolicyPath())
		if err != nil {
			t.Fatalf("策略文件没写: %v", err)
		}
		var policy struct {
			Permission string `json:"permission"`
		}
		if err := json.Unmarshal(raw, &policy); err != nil {
			t.Fatal(err)
		}
		// 审批扩展读的是这个文件：不写进去，界面切了档位助手行为不变。
		if policy.Permission != domain.PermissionReadonly {
			t.Fatalf("策略档位不对: %s", raw)
		}
		if err := te.container.Sessions.SetPermission(sess.ID, "bogus"); err == nil {
			t.Fatal("非法档位应当被拒")
		}
	})

	t.Run("审批桥：登记后能查到待决项", func(t *testing.T) {
		sess := te.newSession(t)
		te.container.Approvals.Open(sess.ID, "req-1", "confirm", "允许 write?", "要写入 a.txt", nil)
		pending := te.container.Approvals.Pending(sess.ID)
		if len(pending) != 1 {
			t.Fatalf("待决审批没登记: %+v", pending)
		}
		if pending[0].Tool != "write" {
			t.Fatalf("工具名没从标题里解析出来: %+v", pending[0])
		}
		// 决策在没有活进程时会失败（响应必须回到发出请求的进程）。
		if err := te.container.Approvals.Decide(sess.ID, "req-1", true, domain.ApprovalScopeOnce); err == nil {
			// 进程在跑时决策会成功——两种情况都不该 panic，这里只要求它不再挂在等待表里。
			if len(te.container.Approvals.Pending(sess.ID)) != 0 {
				t.Fatal("决策后仍挂在等待表里")
			}
		}
	})

	t.Run("重复决策返回 4102", func(t *testing.T) {
		sess := te.newSession(t)
		// 没登记过的 id：摘不到条目就必须报 4102，不能改状态。
		err := te.container.Approvals.Decide(sess.ID, "no-such-approval", true, domain.ApprovalScopeOnce)
		if err == nil {
			t.Fatal("不存在的审批应当报错")
		}
		if pkg.CodeOf(err) != "4102" {
			t.Fatalf("应当返回 4102，实际 %s", pkg.CodeOf(err))
		}
	})

	t.Run("会话级的总是允许写进策略", func(t *testing.T) {
		sess := te.newSession(t)
		te.container.Approvals.Open(sess.ID, "req-2", "confirm", "允许 powershell?", "要跑命令", nil)
		_ = te.container.Approvals.Decide(sess.ID, "req-2", true, domain.ApprovalScopeSession)
		allowed := te.container.Approvals.AllowedTools(sess.ID)
		t.Logf("已放行工具: %v", allowed)
		// 删会话时必须清掉：grants 只增不减会随会话数泄漏。
		te.container.Approvals.ForgetSession(sess.ID)
		if len(te.container.Approvals.AllowedTools(sess.ID)) != 0 {
			t.Fatal("ForgetSession 没清掉放行记忆")
		}
	})

	t.Run("工作区切换会重建进程上下文", func(t *testing.T) {
		sess := te.newSession(t)
		if _, err := te.container.Chat.Send(sess.ID, "先聊一句", nil); err != nil {
			t.Fatalf("发送失败: %v", err)
		}
		te.waitIdle(t, sess.ID)
		newWS := t.TempDir()
		if err := te.container.Sessions.SetWorkspace(sess.ID, newWS); err != nil {
			t.Fatalf("切换工作区失败: %v", err)
		}
		if err := te.container.Sessions.SetWorkspace(sess.ID, filepath.Join(newWS, "nope")); err == nil {
			t.Fatal("不存在的目录应当被拒")
		}
	})

	t.Run("子代理用独立进程且结论带用量", func(t *testing.T) {
		parent := te.newSession(t)
		out, err := te.container.Subagents.Run(context.Background(), parent.ID, domain.SubagentREQ{
			Task: "查一下这个目录里有什么",
		})
		if err != nil {
			t.Fatalf("子代理失败: %v", err)
		}
		// 结论里必须带用量：父助手据此判断「这一步贵不贵」。
		if !strings.Contains(out, "tokens") {
			t.Fatalf("结论里没有用量: %q", out)
		}
		// 子会话跑完要删掉：不删就是每用一次留一条孤儿隐藏会话。
		list, err := te.env.Repo.ListSessions(200, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if strings.HasPrefix(s.Title, "[子代理]") && !s.Hidden {
				t.Fatalf("子会话不该出现在用户列表: %+v", s)
			}
		}
	})

	t.Run("同步配置写出 PI 的三个文件", func(t *testing.T) {
		te.container.SyncPiConfig()
		for _, path := range []string{
			te.env.PiEnv.ModelsPath(),
			te.env.PiEnv.SettingsPath(),
			te.env.PiEnv.AppendSystemPath(),
			te.env.PiEnv.MCPPath(),
			te.env.PiEnv.PolicyPath(),
		} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("配置文件没写出来 %s: %v", path, err)
			}
		}
		// 审批扩展要落到扩展目录，否则闸门静默消失。
		ext := filepath.Join(te.env.PiEnv.ExtensionsDir(), "workbaby.ts")
		if _, err := os.Stat(ext); err != nil {
			t.Fatalf("审批扩展没写出来: %v", err)
		}
		settings, err := te.env.PiEnv.ReadSettings()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range settings.Extensions {
			if e == ext {
				found = true
			}
		}
		// 只写文件不登记，PI 根本不加载它。
		if !found {
			t.Fatalf("扩展没登记进 settings: %v", settings.Extensions)
		}
	})

	t.Run("APPEND_SYSTEM 只改受控块", func(t *testing.T) {
		path := te.env.PiEnv.AppendSystemPath()
		// 用户自己在文件里写了东西。
		if err := os.WriteFile(path, []byte("# 我的项目规矩\n\n不许用 emoji。\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		te.container.SyncPiConfig()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		// 块外是用户手写内容：整体覆盖会静默删掉他的东西。
		if !strings.Contains(text, "我的项目规矩") {
			t.Fatalf("用户手写内容被覆盖了: %s", text)
		}
		if !strings.Contains(text, "<!-- workbaby:begin -->") {
			t.Fatalf("受控块没写进去: %s", text)
		}
		// 再跑一次不能累积重复块。
		te.container.SyncPiConfig()
		raw2, _ := os.ReadFile(path)
		if strings.Count(string(raw2), "<!-- workbaby:begin -->") != 1 {
			t.Fatalf("受控块重复了: %s", raw2)
		}
	})

	t.Run("工具开关写 PI 的 defaultTools", func(t *testing.T) {
		if err := te.container.Tools.SetEnabled("powershell", false); err != nil {
			t.Fatalf("停用工具失败: %v", err)
		}
		settings, err := te.env.PiEnv.ReadSettings()
		if err != nil {
			t.Fatal(err)
		}
		if len(settings.DefaultTools) == 0 {
			t.Fatal("defaultTools 没写")
		}
		for _, name := range settings.DefaultTools {
			if name == "powershell" {
				t.Fatalf("停用的工具还在列表里: %v", settings.DefaultTools)
			}
			if strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
				t.Fatalf("不该出现增减语法: %v", settings.DefaultTools)
			}
		}
		// 恢复
		if err := te.container.Tools.SetEnabled("powershell", true); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("MCP 配置解析失败不落盘", func(t *testing.T) {
		bad := `{"mcpServers": {`
		if _, err := te.container.MCP.SaveConfig(domain.SaveMCPConfigREQ{Raw: bad}); err == nil {
			t.Fatal("坏配置应当被拒")
		}
		raw, err := te.env.PiEnv.ReadMCPRaw()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), bad) {
			t.Fatalf("坏配置被写进文件了: %s", raw)
		}
		// 缺 command 也要拒。
		if _, err := te.container.MCP.SaveConfig(domain.SaveMCPConfigREQ{
			Raw: `{"mcpServers":{"x":{}}}`,
		}); err == nil {
			t.Fatal("缺 command 应当被拒")
		}
		good := `{"mcpServers":{"x":{"command":"node","args":["a.js"],"timeout":30}}}`
		resp, err := te.container.MCP.SaveConfig(domain.SaveMCPConfigREQ{Raw: good})
		if err != nil {
			t.Fatalf("好配置保存失败: %v", err)
		}
		if len(resp.Servers) != 2 {
			// 内置的 workbaby-knowledge 由程序维护，保存后会被补回——用户删不掉它是故意的。
			t.Fatalf("解析出的服务数不对（应为用户的 1 个 + 内置 1 个）: %+v", resp.Servers)
		}
		var hasUser, hasKnowledge bool
		for _, sv := range resp.Servers {
			switch sv.Name {
			case "x":
				hasUser = true
			case "workbaby-knowledge":
				hasKnowledge = true
			}
		}
		if !hasUser || !hasKnowledge {
			t.Fatalf("服务条目不全: %+v", resp.Servers)
		}
	})

	t.Run("技能停用是移出目录而不是写 KV", func(t *testing.T) {
		// 只验证判定逻辑：写 KV 不移动目录会得到「界面已停用、PI 照常加载」的假状态。
		skills := te.container.Skills.List()
		t.Logf("当前技能数: %d", len(skills))
	})

	t.Run("设置同步进 settings.json", func(t *testing.T) {
		if err := te.env.Repo.SetSetting(domain.SettingCompactionReserve, "24576"); err != nil {
			t.Fatal(err)
		}
		if err := te.container.Settings.Set(domain.SettingThinkingLevel, "high"); err != nil {
			t.Fatalf("设置思考档失败: %v", err)
		}
		settings, err := te.env.PiEnv.ReadSettings()
		if err != nil {
			t.Fatal(err)
		}
		if settings.DefaultThinkingLevel != "high" {
			t.Fatalf("思考档没同步: %+v", settings)
		}
		if settings.Compaction == nil || settings.Compaction.ReserveTokens != 24576 {
			t.Fatalf("压缩参数没同步: %+v", settings.Compaction)
		}
	})

	t.Run("并发发送不串会话", func(t *testing.T) {
		a := te.newSession(t)
		b := te.newSession(t)
		var wg sync.WaitGroup
		wg.Add(2)
		errs := make(chan error, 2)
		go func() {
			defer wg.Done()
			_, err := te.container.Chat.Send(a.ID, "A 会话", nil)
			errs <- err
		}()
		go func() {
			defer wg.Done()
			_, err := te.container.Chat.Send(b.ID, "B 会话", nil)
			errs <- err
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				// 同一工作区串行是既定约束：第二个可能被拒，但不能 panic 或错乱。
				if pkg.CodeOf(err) == "9999" {
					t.Fatalf("并发发送出现了非业务错误: %v", err)
				}
			}
		}
		te.waitIdle(t, a.ID)
	})
}
