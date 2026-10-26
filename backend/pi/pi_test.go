// PI 监督链路：真实子进程 + 真实 stdin/stdout 分帧，覆盖编解码、事件顺序与生命周期。
// 坏了的表现：界面一直转圈、done 提前到、进程死了没反应、提问答不回去。
package pi

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/pi/piptest"
)

// recorder 收集 handler 收到的全部事件，供断言。
type recorder struct {
	mu      sync.Mutex
	events  []Event
	exited  bool
	exitErr error
}

func (r *recorder) OnEvent(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recorder) OnExit(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exited = true
	r.exitErr = err
}

func (r *recorder) count(kind EventKind) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func (r *recorder) findFunc(kind EventKind, match func(Event) bool) (Event, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Kind == kind && (match == nil || match(e)) {
			return e, true
		}
	}
	return Event{}, false
}

func (r *recorder) isExited() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exited
}

// start 拉起一个假 PI 并等到它开始吐事件（首帧扩展请求到了 = 读循环已跑起来）。
func start(t *testing.T, exe, scenario string) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{}
	client, err := Start(Config{
		Exe:  exe,
		Env:  os.Environ(),
		Args: []string{"--scenario", scenario},
	}, rec)
	if err != nil {
		t.Fatalf("拉起假 PI 失败: %v", err)
	}
	t.Cleanup(client.Stop)
	waitFor(t, func() bool { return rec.count(KindExtensionUI) >= 1 })
	return client, rec
}

// waitFor 轮询等待条件成立，超时即判定链路卡住。
// 假进程 5 秒足够；真内核冷启动要几十秒，用 waitQuiet 给更长预算。
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	if !waitQuiet(t, cond, 5*time.Second) {
		t.Fatal("等待超时：链路没有按预期推进")
	}
}

// waitQuiet 是 waitFor 的可超时版本。
func waitQuiet(t *testing.T, cond func() bool, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return cond()
}

// TestPiChain 是 pi 包唯一的测试入口。五个子测试按「协议 → 事件 → 交互 → 生命周期 → 真内核」排列。
func TestPiChain(t *testing.T) {
	exe := piptest.Build(t)

	// 协议层一条链验完：分帧、未知记录、按 id 关联、fire-and-forget 字段——坏了都是「命令全部超时」。
	t.Run("协议：分帧、未知记录、按 id 关联与扩展请求字段", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		// 假 PI 首帧就发两条 setStatus（中间隔着 U+2028）：都到齐才算分帧正确。
		waitFor(t, func() bool { return rec.count(KindExtensionUI) >= 2 })
		// 连续两条不同命令，响应必须各归各的：按到达顺序配对会串命令。
		st, err := client.GetState(context.Background())
		if err != nil {
			t.Fatalf("遇到扩展 UI 记录后命令应当照常可用: %v", err)
		}
		if st.SessionID == "" || st.SessionFile == "" {
			t.Fatalf("会话标识没拿到: %+v", st)
		}
		stats, err := client.GetSessionStats(context.Background())
		if err != nil {
			t.Fatalf("get_session_stats 失败: %v", err)
		}
		if stats.TotalTokens() != 1200 || stats.ContextTokens() != 1000 {
			t.Fatalf("统计读串了: %+v", stats)
		}

		// notify / setStatus / setWidget / setTitle 四种 fire-and-forget：
		// 字段解析丢一个，界面上就少一种反馈；ANSI 色码协议层不剥（service 翻译时才剥）。
		nc, nrec := start(t, exe, "notify")
		if _, err := nc.Prompt(context.Background(), "打招呼", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitExtension := func(id string) Event {
			waitFor(t, func() bool {
				_, ok := nrec.findFunc(KindExtensionUI, func(e Event) bool { return e.ID == id })
				return ok
			})
			ev, _ := nrec.findFunc(KindExtensionUI, func(e Event) bool { return e.ID == id })
			return ev
		}
		if ev := waitExtension("n1"); ev.Method != "notify" || ev.Body != "命令被用户拒绝" || ev.NotifyType != "warning" {
			t.Fatalf("notify 字段不对: %+v", ev)
		}
		if ev := waitExtension("n2"); ev.Method != "setStatus" ||
			ev.StatusText != "\x1b[39m\x1b[2mTurn 3 running...\x1b[22;39m" {
			t.Fatalf("setStatus 字段不对: %+v", ev)
		}
		if ev := waitExtension("n3"); ev.Method != "setWidget" || len(ev.WidgetLines) != 2 {
			t.Fatalf("setWidget 字段不对: %+v", ev)
		}
		if ev := waitExtension("n4"); ev.Method != "setTitle" || ev.Title != "pi - demo" {
			t.Fatalf("setTitle 字段不对: %+v", ev)
		}
		// fire-and-forget 不需要应答，也绝不能堵住命令通道。
		if _, err := nc.GetState(context.Background()); err != nil {
			t.Fatalf("fire-and-forget 之后命令通道被堵: %v", err)
		}
	})

	// 事件顺序与翻译层一起验：一轮对话里同时发生增量、工具调用、压缩与自动重试，
	// 任何一段的字段或顺序还原错了，界面上就是「正文少一段」「工具没输出」或「还在重试却说完成」。
	t.Run("事件与翻译：顺序、增量、工具结果与压缩", func(t *testing.T) {
		client, rec := start(t, exe, "tools")
		if _, err := client.Prompt(context.Background(), "读文件", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindToolEnd) >= 1 })
		var text, thinking bool
		rec.mu.Lock()
		for _, e := range rec.events {
			if e.Kind != KindMessageUpdate {
				continue
			}
			switch e.DeltaType {
			case "text_delta":
				text = text || e.Delta != ""
			case "thinking_delta":
				thinking = thinking || e.Delta != ""
			}
		}
		rec.mu.Unlock()
		if !text || !thinking {
			t.Fatalf("增量没解析出来（text=%v thinking=%v）", text, thinking)
		}
		ev, _ := rec.findFunc(KindToolEnd, nil)
		if ev.Result == nil || len(ev.Result.Content) == 0 || ev.Result.Content[0].Text != "文件内容" ||
			ev.DurationMs != 12 || ev.IsError {
			t.Fatalf("工具结果与元数据没还原: %+v", ev)
		}

		// 压缩：前后的 token 量是「上下文省了多少」的唯一来源。
		cc, crec := start(t, exe, "compact")
		if _, err := cc.Prompt(context.Background(), "聊聊", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return crec.count(KindCompactionEnd) >= 1 })
		cev, _ := crec.findFunc(KindCompactionEnd, nil)
		if cev.CompactResult == nil || cev.CompactResult.TokensBefore != 9000 || cev.CompactResult.EstimatedTokensAfter != 2000 {
			t.Fatalf("压缩结果没解析出来: %+v", cev.CompactResult)
		}

		// 顺序：done 只挂 agent_settled——自动重试 / 压缩都发生在 agent_end 之后，
		// done 挂早了会在还在重试时报「已完成」。
		bc, brec := start(t, exe, "basic")
		if _, err := bc.Prompt(context.Background(), "你好", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return brec.count(KindAgentSettled) >= 1 })
		brec.mu.Lock()
		kinds := make([]EventKind, 0, len(brec.events))
		for _, e := range brec.events {
			kinds = append(kinds, e.Kind)
		}
		brec.mu.Unlock()
		idxEnd, idxRetry, idxSettled := -1, -1, -1
		for i, k := range kinds {
			switch k {
			case KindAgentEnd:
				idxEnd = i
			case KindRetryStart:
				idxRetry = i
			case KindAgentSettled:
				idxSettled = i
			}
		}
		if idxEnd < 0 || idxRetry < 0 || idxSettled < 0 || !(idxEnd < idxRetry && idxRetry < idxSettled) {
			t.Fatalf("事件顺序不对（agent_end → retry → settled）: %v", kinds)
		}
	})

	// 提问桥的双向：confirm 字段完整解析，响应必须写回同一个 id；input 的 placeholder / timeout 不许丢。
	// 坏了 = 界面弹了提问、点了确认却石沉大海，助手永远等在那里。
	t.Run("提问桥：请求解析与按 id 回写", func(t *testing.T) {
		client, rec := start(t, exe, "approval")
		if _, err := client.Prompt(context.Background(), "写文件", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool {
			_, ok := rec.findFunc(KindExtensionUI, func(e Event) bool { return e.Method == "confirm" })
			return ok
		})
		// 首帧还有 setStatus：找到的必须是那条 confirm，而不是第一条扩展请求。
		found, _ := rec.findFunc(KindExtensionUI, func(e Event) bool { return e.Method == "confirm" })
		if found.ID != "confirm-1" || found.Title != "允许 write?" || found.Body != "要写入文件 a.txt" {
			t.Fatalf("提问请求字段不对: %+v", found)
		}
		if err := client.WriteResponse(map[string]any{
			"type": "extension_ui_response", "id": "confirm-1", "confirmed": true,
		}); err != nil {
			t.Fatalf("回提问响应失败: %v", err)
		}
		waitFor(t, func() bool {
			_, ok := rec.findFunc(KindExtensionUI, func(e Event) bool { return e.ID == "input-1" })
			return ok
		})
		typed, _ := rec.findFunc(KindExtensionUI, func(e Event) bool { return e.ID == "input-1" })
		if typed.Method != "input" || typed.Placeholder != "1,3; 自定义答案" || typed.TimeoutMs != 30000 {
			t.Fatalf("input 提问的 placeholder / timeout 没解析: %+v", typed)
		}
	})

	// 生命周期：有序退出、崩溃报错、死进程命令立刻失败、一个工作区一个进程。
	// 坏了 = 关窗之后进程留在后台、界面在死进程上等超时、两个工作区串了会话。
	t.Run("生命周期：退出、崩溃与工作区归属", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		client.Stop()
		waitFor(t, func() bool { return rec.isExited() })
		if client.Alive() {
			t.Fatal("停止后进程还活着")
		}

		crashed := &recorder{}
		bad, err := Start(Config{Exe: exe, Env: os.Environ(), Args: []string{"--scenario", "crash"}}, crashed)
		if err != nil {
			t.Fatalf("拉起假 PI 失败: %v", err)
		}
		t.Cleanup(bad.Stop)
		waitFor(t, func() bool { return crashed.isExited() })
		if bad.Alive() {
			t.Fatal("崩溃后仍报告存活")
		}
		// 死进程上的命令必须立刻失败，不能等超时。
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := bad.GetState(ctx); err == nil {
			t.Fatal("死进程上的命令应当报错")
		}

		sup := NewSupervisor(
			func(workspace string) (Config, error) {
				return Config{Exe: exe, Env: os.Environ()}, nil
			},
			func(workspace string) Handler { return &recorder{} },
		)
		// 进程占着那个目录（它是 cwd），必须先停进程再让 TempDir 清目录。
		t.Cleanup(sup.StopAll)

		if _, err := sup.Ensure(context.Background(), ""); err == nil {
			t.Fatal("空工作区应当报错")
		}
		ws := t.TempDir()
		a, err := sup.Ensure(context.Background(), ws)
		if err != nil {
			t.Fatalf("首次 Ensure 失败: %v", err)
		}
		// 同一工作区必须复用：每次发消息都 spawn 一个新的会堆满进程。
		if b, err := sup.Ensure(context.Background(), ws); err != nil || b != a {
			t.Fatalf("同一工作区应当复用同一个进程: %v", err)
		}
		// 换工作区必须换进程：cwd 是进程级属性。
		other := t.TempDir()
		if c, err := sup.Ensure(context.Background(), other); err != nil || c == a {
			t.Fatalf("不同工作区不能共用一个进程: %v", err)
		}
		// 先停掉进程，TempDir 的清理才不会撞上「目录被占用」。
		sup.StopAll()
	})

}
