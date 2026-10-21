// PI 监督链路：真实子进程 + 真实 stdin/stdout 分帧，覆盖编解码、事件顺序与生命周期。
// 坏了的表现：界面一直转圈、done 提前到、进程死了没反应、停止时排队的句子凭空消失、
// 本机装了 PI 却起不来（入口解析错、就绪握手超时、第二轮对话换错了进程）。
package pi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func (r *recorder) kinds() []EventKind {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]EventKind, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.Kind)
	}
	return out
}

func (r *recorder) count(kind EventKind) int {
	n := 0
	for _, k := range r.kinds() {
		if k == kind {
			n++
		}
	}
	return n
}

func (r *recorder) find(kind EventKind) (Event, bool) {
	return r.findFunc(kind, nil)
}

// findFunc 按种类 + 谓词找第一条匹配的事件；谓词为 nil 时等价于 find。
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

// start 拉起一个假 PI 并等到它开始吐事件。
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
	// 首帧扩展请求到了就说明读循环已经跑起来了。
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

// TestPiChain 是 pi 包唯一的测试入口。
func TestPiChain(t *testing.T) {
	exe := piptest.Build(t)

	// 协议分帧与命令关联：坏帧不挡命令、响应按 id 各归各的。
	// 分帧按 U+2028 切会把首帧截成半截 JSON；响应按到达顺序配对会串命令。
	t.Run("协议分帧与命令响应解码", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		// 两条 setStatus 都要到：按 U+2028 切分会把第二条截成半截 JSON 而丢掉。
		waitFor(t, func() bool { return rec.count(KindExtensionUI) >= 2 })
		// 两条 setStatus 都要到：按 U+2028 切分会把第二条截成半截 JSON 而丢掉。
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
		if stats.TotalTokens != 1200 {
			t.Fatalf("统计读串了: %+v", stats)
		}
	})

	// 事件顺序是界面对账的根基：done 挂早了会在还在重试时报「已完成」。
	t.Run("事件顺序：agent_end → 重试 → settled，done 只挂在最后", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		if _, err := client.Prompt(context.Background(), "你好", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindAgentSettled) >= 1 })
		kinds := rec.kinds()
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

	// 翻译层：协议记录 → 界面形态。一轮里同时发生工具、增量、压缩。
	// 任何一段的字段还原错了，界面上就是「工具没输出」或「压缩了但没提示」。
	t.Run("翻译层：工具结果与增量、压缩前后量都对", func(t *testing.T) {
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
		ev, _ := rec.find(KindToolEnd)
		if ev.Result == nil || len(ev.Result.Content) == 0 || ev.Result.Content[0].Text != "文件内容" ||
			ev.DurationMs != 12 || ev.IsError {
			t.Fatalf("工具结果与元数据没还原: %+v", ev)
		}

		cc, crec := start(t, exe, "compact")
		if _, err := cc.Prompt(context.Background(), "聊聊", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return crec.count(KindCompactionEnd) >= 1 })
		cev, _ := crec.find(KindCompactionEnd)
		if cev.CompactResult == nil || cev.CompactResult.TokensBefore != 9000 || cev.CompactResult.EstimatedTokensAfter != 2000 {
			t.Fatalf("压缩结果没解析出来: %+v", cev.CompactResult)
		}
	})

	// fire-and-forget 四种：字段完整解析、无需应答、不阻塞命令通道。
	t.Run("即时反馈：四种扩展请求字段完整且不堵命令通道", func(t *testing.T) {
		fc, frec := start(t, exe, "notify")
		if _, err := fc.Prompt(context.Background(), "打招呼", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitExtension := func(id string) Event {
			waitFor(t, func() bool {
				_, ok := frec.findFunc(KindExtensionUI, func(e Event) bool { return e.ID == id })
				return ok
			})
			ev, _ := frec.findFunc(KindExtensionUI, func(e Event) bool { return e.ID == id })
			return ev
		}
		if ev := waitExtension("n1"); ev.Method != "notify" || ev.Body != "命令被用户拒绝" || ev.NotifyType != "warning" {
			t.Fatalf("notify 字段不对: %+v", ev)
		}
		if ev := waitExtension("n2"); ev.Method != "setStatus" || ev.StatusKey != "my-ext" ||
			// 状态文字带终端色码：协议层不剥，原样传给上层（service 翻译时才剥）。
			ev.StatusText != "\x1b[39m\x1b[2mTurn 3 running...\x1b[22;39m" {
			t.Fatalf("setStatus 字段不对: %+v", ev)
		}
		if ev := waitExtension("n3"); ev.Method != "setWidget" || len(ev.WidgetLines) != 2 || ev.WidgetLines[0] != "--- My Widget ---" {
			t.Fatalf("setWidget 字段不对: %+v", ev)
		}
		if ev := waitExtension("n4"); ev.Method != "setTitle" || ev.Title != "pi - demo" {
			t.Fatalf("setTitle 字段不对: %+v", ev)
		}
		if _, err := fc.GetState(context.Background()); err != nil {
			t.Fatalf("fire-and-forget 之后命令通道被堵: %v", err)
		}
	})

	// 提问桥的双向：请求字段完整解析，响应必须写回同一个 stdin。
	t.Run("提问请求解析与回写响应", func(t *testing.T) {
		client, rec := start(t, exe, "approval")
		if _, err := client.Prompt(context.Background(), "写文件", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			for _, e := range rec.events {
				if e.Kind == KindExtensionUI && e.Method == "confirm" {
					return true
				}
			}
			return false
		})
		found, _ := rec.findFunc(KindExtensionUI, func(e Event) bool { return e.Method == "confirm" })
		// 首帧还有 setStatus：找到的必须是那条 confirm，而不是第一条扩展请求。
		if found.ID != "confirm-1" || found.Title != "允许 write?" || found.Body != "要写入文件 a.txt" {
			t.Fatalf("提问请求字段不对: %+v", found)
		}
		// message 字段在扩展请求里是正文而不是消息对象，Response 必须写回同一个 id。
		if err := client.WriteResponse(map[string]any{
			"type": "extension_ui_response", "id": "confirm-1", "confirmed": true,
		}); err != nil {
			t.Fatalf("回提问响应失败: %v", err)
		}

		// 多选 / 自定义答案靠 input 的 placeholder 与 timeout 传给用户，丢一个就不成。
		waitFor(t, func() bool {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			for _, e := range rec.events {
				if e.ID == "input-1" {
					return true
				}
			}
			return false
		})
		typed, _ := rec.findFunc(KindExtensionUI, func(e Event) bool { return e.ID == "input-1" })
		if typed.Method != "input" || typed.Placeholder != "1,3; 自定义答案" || typed.TimeoutMs != 30000 {
			t.Fatalf("input 提问的 placeholder / timeout 没解析: %+v", typed)
		}
	})

	// 生命周期：有序退出、崩溃报错、死进程命令立刻失败、一个工作区一个进程。
	t.Run("生命周期与进程归属", func(t *testing.T) {
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

	// 真内核链路：只在 -short 之外跑（起真的 PI，一轮对话要几十秒）。
	// 假进程验协议形状，这条验「本机这份 PI 真的能对话」——两者坏的地方不一样：
	// 假进程坏在编解码，真内核坏在入口解析、就绪握手、进程复用与会话落盘。
	t.Run("本机 PI 真实回合：就绪、建会话、两轮对话与进程复用", func(t *testing.T) {
		checkLocalPITurn(t)
	})
}

// checkLocalPITurn 拿本机装的 PI 跑两轮真实对话。
// 入口经 npm 的 Windows shim 解析成 node + 脚本：经 cmd.exe 起进程会弹控制台窗口。
func checkLocalPITurn(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("真内核链路只在 -short 之外跑")
	}
	pienv := localPiEnv(t)
	ws := t.TempDir()

	rec := &recorder{}
	client, err := Start(Config{
		Exe: pienv.exe, Args: pienv.args, Workspace: ws, Env: pienv.env,
	}, rec)
	if err != nil {
		t.Fatalf("起不了 PI 进程: %v", err)
	}
	t.Cleanup(client.Stop)

	// 立刻就死说明入口或参数不对：stderr 里带着真实原因。
	time.Sleep(3 * time.Second)
	if !client.Alive() {
		t.Fatalf("PI 进程启动后立刻退出（stderr: %s）", client.Stderr())
	}

	ctx := context.Background()
	// 就绪：真进程冷启动要加载扩展，get_state 重试到通为止。
	var st *State
	for i := 0; i < 60; i++ {
		if st, err = client.GetState(ctx); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("PI 一直没就绪: %v（stderr: %s）", err, client.Stderr())
	}
	if st.Model.ID == "" {
		t.Fatal("get_state 没带回模型：models.json 或登录态有问题")
	}
	// 建一条真会话：会话指针与落盘文件是界面会话索引的两条命脉。
	if err := client.NewSession(ctx, ""); err != nil {
		t.Fatalf("new_session 失败: %v", err)
	}
	if st, err = client.GetState(ctx); err != nil {
		t.Fatalf("新会话后取状态失败: %v", err)
	}
	if st.SessionID == "" || st.SessionFile == "" {
		t.Fatalf("新会话没有落地: %+v", st)
	}
	sessionID, sessionFile := st.SessionID, st.SessionFile

	for turn := 1; turn <= 2; turn++ {
		if _, err := client.Prompt(ctx, fmt.Sprintf("只回一个字：第%d轮", turn), "", nil); err != nil {
			t.Fatalf("第 %d 轮 prompt 被拒绝: %v", turn, err)
		}
		// PI 起完扩展才开始跑：给足等待，一条事件都没有就是链路断了。
		if !waitQuiet(t, func() bool { return rec.count(KindAgentSettled) >= turn }, 180*time.Second) {
			t.Fatalf("第 %d 轮没跑完（stderr: %s）", turn, client.Stderr())
		}
	}

	// 进程复用：第二个 prompt 不允许换进程，指针仍停在同一条会话上。
	st2, err := client.GetState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st2.SessionID != sessionID || st2.SessionFile != sessionFile {
		t.Fatalf("会话指针漂了: %s -> %s", sessionID, st2.SessionID)
	}
	if _, err := os.Stat(sessionFile); err != nil {
		t.Fatalf("会话文件没落盘: %v", err)
	}
}

// localPiEnv 解析本机 PI 的启动入口与进程环境；没装 PI 就跳过这条链。
// 入口必须还原成 node + 脚本：经 cmd.exe 起进程会弹控制台窗口，且 CREATE_NO_WINDOW 只作用于 cmd.exe。
func localPiEnv(t *testing.T) struct {
	exe  string
	args []string
	env  []string
} {
	t.Helper()
	shim, err := exec.LookPath("pi.cmd")
	if err != nil {
		shim, err = exec.LookPath("pi.exe")
	}
	if err != nil {
		t.Skip("本机没有 PI")
	}
	dir := filepath.Dir(shim)
	script := filepath.Join(dir, `node_modules\@earendil-works\pi-coding-agent\dist\bundle\cli.js`)
	if _, err := os.Stat(script); err != nil {
		t.Skip("找不到 PI 的入口脚本")
	}
	node := filepath.Join(dir, "node.exe")
	if _, err := os.Stat(node); err != nil {
		t.Skip("找不到 node.exe")
	}
	// 前置拼接内置 node：后置会被用户 PATH 上的旧 node 抢先。
	env := append([]string{"PATH=" + dir + ";" + os.Getenv("PATH")}, os.Environ()...)
	return struct {
		exe  string
		args []string
		env  []string
	}{exe: node, args: []string{script, "--mode", "rpc"}, env: env}
}
