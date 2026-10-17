// PI 监督链路：真实子进程 + 真实 stdin/stdout 分帧，覆盖编解码、命令关联与生命周期。
// 坏了的表现：事件串不起来（界面一直转圈）、done 提前到（自动重试期间显示已完成）、
// 进程死了不报错（用户只看到「没反应」）、停止时排队的句子凭空消失。
package pi

import (
	"context"
	"encoding/json"
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
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Kind == kind {
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

	t.Run("按 LF 分帧且 U+2028 不截断记录", func(t *testing.T) {
		_, rec := start(t, exe, "basic")
		// 两条 setStatus 都要到：按 U+2028 切分会把第二条截成半截 JSON 而丢掉。
		waitFor(t, func() bool { return rec.count(KindExtensionUI) >= 2 })
	})

	t.Run("未知记录忽略不报错", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		// 首帧是 setStatus：如果它被当成错误，后面的命令全部会挂。
		if _, err := client.GetState(context.Background()); err != nil {
			t.Fatalf("遇到扩展 UI 记录后命令应当照常可用: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindExtensionUI) >= 2 })
	})

	t.Run("命令按 id 关联且拿到数据", func(t *testing.T) {
		client, _ := start(t, exe, "basic")
		st, err := client.GetState(context.Background())
		if err != nil {
			t.Fatalf("get_state 失败: %v", err)
		}
		if st.SessionID == "" || st.SessionFile == "" {
			t.Fatalf("会话标识没拿到: %+v", st)
		}
		// 连续发两条不同命令，响应不能串。
		stats, err := client.GetSessionStats(context.Background())
		if err != nil {
			t.Fatalf("get_session_stats 失败: %v", err)
		}
		if stats.TotalTokens != 1200 {
			t.Fatalf("统计读串了: %+v", stats)
		}
	})

	t.Run("done 只在 agent_settled 之后", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		if _, err := client.Prompt(context.Background(), "你好", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindAgentSettled) >= 1 })
		// agent_end 必须先到，且它之后还有重试事件——把 done 挂在 agent_end
		// 就会在「还在重试」的时候告诉用户已完成。
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

	t.Run("流式增量与工具事件都能收到", func(t *testing.T) {
		client, rec := start(t, exe, "tools")
		if _, err := client.Prompt(context.Background(), "读文件", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindToolEnd) >= 1 })
		// 正文与思考两类增量都要单独解析出来。
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
	})

	t.Run("工具结果按内容块还原", func(t *testing.T) {
		client, rec := start(t, exe, "tools")
		if _, err := client.Prompt(context.Background(), "读文件", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindToolEnd) >= 1 })
		ev, _ := rec.find(KindToolEnd)
		if ev.Result == nil || len(ev.Result.Content) == 0 || ev.Result.Content[0].Text != "文件内容" {
			t.Fatalf("工具结果没还原: %+v", ev.Result)
		}
		if ev.DurationMs != 12 || ev.IsError {
			t.Fatalf("工具元数据不对: %+v", ev)
		}
	})

	t.Run("压缩事件带前后 token", func(t *testing.T) {
		client, rec := start(t, exe, "compact")
		if _, err := client.Prompt(context.Background(), "聊聊", "", nil); err != nil {
			t.Fatalf("prompt 失败: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindCompactionEnd) >= 1 })
		ev, _ := rec.find(KindCompactionEnd)
		if ev.CompactResult == nil {
			t.Fatal("压缩结果没解析出来")
		}
		if ev.CompactResult.TokensBefore != 9000 || ev.CompactResult.EstimatedTokensAfter != 2000 {
			t.Fatalf("压缩前后数字不对: %+v", ev.CompactResult)
		}
	})

	t.Run("审批请求能带出标题与正文", func(t *testing.T) {
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
		rec.mu.Lock()
		var found Event
		for _, e := range rec.events {
			if e.Kind == KindExtensionUI && e.Method == "confirm" {
				found = e
				break
			}
		}
		rec.mu.Unlock()
		if found.ID != "confirm-1" || found.Title != "允许 write?" {
			t.Fatalf("审批请求字段不对: %+v", found)
		}
		// message 字段在扩展请求里是正文而不是消息对象。
		if found.Body != "要写入文件 a.txt" {
			t.Fatalf("审批正文没解析出来: %q", found.Body)
		}
	})

	t.Run("回审批响应不报错", func(t *testing.T) {
		client, _ := start(t, exe, "approval")
		if err := client.WriteResponse(map[string]any{
			"type": "extension_ui_response", "id": "confirm-1", "confirmed": true,
		}); err != nil {
			t.Fatalf("回审批响应失败: %v", err)
		}
	})

	t.Run("停止先取回排队文本", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		q, err := client.ClearQueue(context.Background())
		if err != nil {
			t.Fatalf("clear_queue 失败: %v", err)
		}
		// 不取回就 abort，用户排队的那句话会凭空消失。
		if len(q.Steering) != 1 || q.Steering[0] != "排队的这句话" {
			t.Fatalf("排队文本没取回: %+v", q)
		}
		if err := client.Abort(context.Background()); err != nil {
			t.Fatalf("abort 失败: %v", err)
		}
		waitFor(t, func() bool { return rec.count(KindAgentSettled) >= 1 })
	})

	t.Run("关 stdin 后有序退出", func(t *testing.T) {
		client, rec := start(t, exe, "basic")
		client.Stop()
		waitFor(t, func() bool { return rec.isExited() })
		if client.Alive() {
			t.Fatal("停止后进程还活着")
		}
	})

	t.Run("进程崩溃后报错且不自动重启", func(t *testing.T) {
		rec := &recorder{}
		client, err := Start(Config{Exe: exe, Env: os.Environ(), Args: []string{"--scenario", "crash"}}, rec)
		if err != nil {
			t.Fatalf("拉起假 PI 失败: %v", err)
		}
		t.Cleanup(client.Stop)
		waitFor(t, func() bool { return rec.isExited() })
		if client.Alive() {
			t.Fatal("崩溃后仍报告存活")
		}
		// 死进程上的命令必须立刻失败，不能等超时。
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, err := client.GetState(ctx); err == nil {
			t.Fatal("死进程上的命令应当报错")
		}
	})

	t.Run("ResolveExe 认出 pi.cmd", func(t *testing.T) {
		dir := t.TempDir()
		if got := ResolveExe(dir); got != "" {
			t.Fatalf("空目录不该找到可执行文件: %s", got)
		}
		file := filepath.Join(dir, "pi.cmd")
		if err := os.WriteFile(file, []byte("@echo off\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := ResolveExe(dir); got != file {
			t.Fatalf("应当认出 pi.cmd: got %q want %q", got, file)
		}
	})

	t.Run("Supervisor 一个工作区一个进程", func(t *testing.T) {
		sup := NewSupervisor(
			func(workspace string) (Config, error) {
				return Config{Exe: exe, Env: os.Environ()}, nil
			},
			func(workspace string) Handler { return &recorder{} },
		)
		// 进程占着那个目录（它是 cwd），必须先停进程再让 TempDir 清目录。
		t.Cleanup(sup.StopAll)

		ws := t.TempDir()
		a, err := sup.Ensure(context.Background(), ws)
		if err != nil {
			t.Fatalf("首次 Ensure 失败: %v", err)
		}
		b, err := sup.Ensure(context.Background(), ws)
		if err != nil {
			t.Fatalf("二次 Ensure 失败: %v", err)
		}
		// 同一工作区必须复用：每次发消息都 spawn 一个新的会堆满进程。
		if a != b {
			t.Fatal("同一工作区应当复用同一个进程")
		}
		if a.PID() == 0 {
			t.Fatal("进程号为空")
		}
		// 换工作区必须换进程：cwd 是进程级属性。
		other := t.TempDir()
		c, err := sup.Ensure(context.Background(), other)
		if err != nil {
			t.Fatalf("换工作区 Ensure 失败: %v", err)
		}
		if c == a {
			t.Fatal("不同工作区不能共用一个进程")
		}
		// 先停掉进程，TempDir 的清理才不会撞上「目录被占用」。
		sup.StopAll()
	})

	t.Run("空工作区拒绝拉起", func(t *testing.T) {
		sup := NewSupervisor(
			func(workspace string) (Config, error) { return Config{Exe: exe, Env: os.Environ()}, nil },
			func(workspace string) Handler { return &recorder{} },
		)
		t.Cleanup(sup.StopAll)
		if _, err := sup.Ensure(context.Background(), ""); err == nil {
			t.Fatal("空工作区应当报错（没有 cwd 就没法跑）")
		}
	})
}

// 编译期确认 json 被引用（事件断言里会用到原始记录）。
var _ = json.Marshal
