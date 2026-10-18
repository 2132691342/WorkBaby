// PI 监督链路：真实子进程 + 真实 stdin/stdout 分帧，覆盖编解码、事件顺序与生命周期。
// 坏了的表现：界面一直转圈、done 提前到、进程死了没反应、停止时排队的句子凭空消失。
package pi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
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

// buildFakePI 编译假 PI 到临时目录，返回可执行路径。
// 编译一次给全部子测试用：每个子测试各编一次会把总时长做乘法。
func buildFakePI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "fakepi.exe")
	cmd := exec.Command("go", "build", "-o", exe, "./backend/pi/pitest")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译假 PI 失败: %v\n%s", err, out)
	}
	return exe
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
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待超时：链路没有按预期推进")
}

// TestPiChain 是 pi 包唯一的测试入口。
func TestPiChain(t *testing.T) {
	exe := buildFakePI(t)

	// 协议分帧与命令关联：坏帧不挡命令、响应按 id 各归各的。
	// 分帧按 U+2028 切会把首帧截成半截 JSON；响应按到达顺序配对会串命令。
	t.Run("协议分帧与命令响应解码", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		// 两条 setStatus 都要到：按 U+2028 切分会把第二条截成半截 JSON 而丢掉。
		waitFor(t, func() bool { return rec.count(KindExtensionUI) >= 2 })
		// 连续两条不同命令，响应必须各归各的。
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
		if _, err := client.AvailableModels(context.Background()); err != nil {
			t.Fatalf("get_available_models 失败: %v", err)
		}
	})

	// 事件顺序是界面对账的根基：done 挂早了会在还在重试时报「已完成」。
	t.Run("事件顺序：done 只在 agent_settled 之后", func(t *testing.T) {
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
		if idxEnd < 0 || idxRetry < 0 || idxSettled < 0 {
			t.Fatalf("事件序列不完整: %v", kinds)
		}
		if !(idxEnd < idxRetry && idxRetry < idxSettled) {
			t.Fatalf("顺序不对（agent_end → retry → settled）: %v", kinds)
		}
	})

	// 翻译层把协议记录还原成界面能用的形态：增量分两类、工具结果带元数据、压缩带前后量。
	t.Run("事件翻译：增量、工具结果与压缩", func(t *testing.T) {
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
		if ev.Result == nil || len(ev.Result.Content) == 0 || ev.Result.Content[0].Text != "文件内容" {
			t.Fatalf("工具结果没还原: %+v", ev.Result)
		}
		if ev.DurationMs != 12 || ev.IsError {
			t.Fatalf("工具元数据不对: %+v", ev)
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
}
