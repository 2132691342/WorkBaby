// 后端加固：设置写入口不能改工作区、会话删除要避让引擎、索引状态要收尾后取样、
// 模板名清洗结果要回给调用方、路径守卫要挡得住符号链接。
// 这几条的共同点是「绕过入口就能改掉别人的事实」，界面看不到也拦不住。
package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/sse"
)

// makeDirLink 建一个指向目录的链接：优先用 junction（不需要开发者模式），
// 失败再退回符号链接。两处参数都逐字直传，不拼 shell 语句。
func makeDirLink(link, target string) error {
	// mklink 是 cmd 的内建命令，只能经 /c 调用；argv 直传，不进 shell。
	if cmd := pkg.LookPath("cmd"); cmd != "" {
		if _, err := pkg.RunProc(cmd, []string{"/c", "mklink", "/J", link, target}, pkg.ProcOptions{TimeoutMs: 10_000}); err == nil {
			if pkg.Exists(link) {
				return nil
			}
		}
	}
	return os.Symlink(target, link)
}

// mustSetWorkspace 走正式入口设工作区。
func mustSetWorkspace(t *testing.T, h *harness, dir string) {
	t.Helper()
	env, code := h.call("POST", "/settings/workspace", map[string]string{"workspace": dir})
	h.mustOK(env, code, "设置工作区", nil)
}

// readSettings 读 GET /settings 的结果。
func readSettings(t *testing.T, h *harness) map[string]string {
	t.Helper()
	env, code := h.call("GET", "/settings", nil)
	var out map[string]string
	h.mustOK(env, code, "读设置", &out)
	return out
}

func TestBackendHardening(t *testing.T) {
	// 设置写入口是「界面能改的键」的边界：工作区只能走专用接口，
	// 否则文件接口的根会被指到任意目录，1002 路径守卫形同虚设。
	t.Run("设置写入口不接受工作区键", func(t *testing.T) {
		h := newHarness(t)
		original := t.TempDir()
		mustSetWorkspace(t, h, original)

		// 复现：直接写 workspace.current 想换根。
		env, _ := h.call("POST", "/settings/save", map[string]string{"workspace.current": "C:\\Windows"})
		if env.Code == pkg.CodeOK {
			t.Fatalf("直接写 workspace.current 应被拒，实际 code=%q", env.Code)
		}
		if env.Code != pkg.CodeBadRequest {
			t.Fatalf("工作区键该回 %q，实际 %q（%s）", pkg.CodeBadRequest, env.Code, env.Message)
		}

		// 历史键同样是工作区状态的另一半，绕过去会让两页对不上。
		env, _ = h.call("POST", "/settings/save", map[string]string{"workspace.history": "[]"})
		if env.Code == pkg.CodeOK {
			t.Fatal("直接写 workspace.history 应被拒")
		}

		// 根没动：文件接口仍被限制在原来的工作区里。
		if got := h.app.Workspace(); got != original {
			t.Fatalf("工作区不该被改写：应为 %q，实际 %q", original, got)
		}
		env, code := h.call("GET", "/files?path="+original, nil)
		h.mustOK(env, code, "列原工作区", nil)

		// 越界读取仍要被拦下：这是「绕过写入口后守卫还在」的证据。
		env, _ = h.call("GET", "/files/read?path="+filepath.Join(original, "..", "outside.txt"), nil)
		if env.Code != pkg.CodePathOutside {
			t.Fatalf("越界读取应回 %q，实际 %q（%s）", pkg.CodePathOutside, env.Code, env.Message)
		}

		// 读回与设置页一致：两处都报同一个工作区，不留「设置页说 A、工作区页说 B」。
		settings := readSettings(t, h)
		if got := settings["workspace.current"]; got != original {
			t.Fatalf("GET /settings 的工作区应与实际一致：应为 %q，实际 %q", original, got)
		}
		env, code = h.call("GET", "/workspaces", nil)
		var list domain.WorkspaceListRESP
		h.mustOK(env, code, "工作区清单", &list)
		if list.Workspace != original {
			t.Fatalf("工作区页应与设置页一致：应为 %q，实际 %q", original, list.Workspace)
		}

		// 正常键照旧写得进去，且读回是同一份。
		env, code = h.call("POST", "/settings/save", map[string]string{"ui.theme": "dark"})
		h.mustOK(env, code, "写普通设置", nil)
		if got := readSettings(t, h)["ui.theme"]; got != "dark" {
			t.Fatalf("普通设置该被保存：实际 %q", got)
		}
	})

	// 密钥不回显：读接口拿到的是掩码，原值只在写路径上。
	t.Run("密钥不回显", func(t *testing.T) {
		h := newHarness(t)
		secret := "search-secret-abcdef"
		if _, err := h.app.Web.SaveSettings(domain.WebSettingsREQ{
			WebKind: "brave", WebApiKey: secret,
		}); err != nil {
			t.Fatalf("保存联网设置失败：%v", err)
		}
		for _, path := range []string{"/settings", "/prefs"} {
			env, code := h.call("GET", path, nil)
			if code != http.StatusOK {
				t.Fatalf("%s：HTTP 应 200，实际 %d", path, code)
			}
			if strings.Contains(string(env.Data), secret) {
				t.Fatalf("%s 回显了密钥原文：%s", path, string(env.Data))
			}
		}
		// 存储里仍是原文：掩码只发生在出口，不能把真值写坏。
		if got, _ := h.store.Get("web.search_key"); got != secret {
			t.Fatalf("存储里该保留密钥原文，实际 %q", got)
		}
	})

	// 当前会话可以直接删：服务先安全解绑（切到新会话）再删文件，
	// 用户不必手动「先切走」；解绑后绑定必须指向另一份会话。
	t.Run("当前会话删除先解绑再删", func(t *testing.T) {
		h := newHarness(t)
		mustSetWorkspace(t, h, t.TempDir())

		env, code := h.call("POST", "/chat/new", nil)
		var created map[string]string
		h.mustOK(env, code, "新建会话", &created)
		file := created["session_file"]
		if file == "" {
			t.Fatal("新建会话应回 session_file")
		}
		if h.app.Chat.CurrentFile() == "" {
			t.Fatal("新建后引擎应绑定该会话")
		}

		env, code = h.call("POST", "/sessions/delete", map[string]string{"session_path": file})
		h.mustOK(env, code, "删除当前会话", nil)
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("当前会话应被删掉：%v", err)
		}
		if next := h.app.Chat.CurrentFile(); next == "" || strings.EqualFold(next, file) {
			t.Fatalf("删除后应绑定到新会话，实际 %q（被删的是 %q）", next, file)
		}
	})

	// 索引重建是后台化的：应答只代表「已受理」，最终状态靠 /index/status 轮询
	// （后台契约钉在 index_lifecycle_test.go）。这里钉的是收尾后的取样：
	// building 已清、索引已建——界面不会永远停在「正在建索引」。
	t.Run("索引重建回收尾后的状态", func(t *testing.T) {
		h := newHarness(t)
		workspace := t.TempDir()
		writeFile(t, filepath.Join(workspace, "main.go"), "package main\n\nfunc Hello() {}\n")
		mustSetWorkspace(t, h, workspace)

		env, code := h.call("POST", "/index/rebuild", nil)
		h.mustOK(env, code, "重建索引", nil)
		status := waitIndexBuilt(t, h)
		if status.IndexBuilding {
			t.Fatal("收尾后 index_building 应为 false：building 必须在取状态之前清掉")
		}
		if !status.IndexBuilt || status.IndexDocCount == 0 {
			t.Fatalf("重建后应报已建索引且文档数大于 0：%+v", status)
		}
	})

	// 模板名会被清洗：清洗结果必须回给调用方，否则拿原文再删就删不掉。
	t.Run("模板保存回清洗后的名字", func(t *testing.T) {
		h := newHarness(t)
		env, code := h.call("POST", "/templates/save", domain.PromptTemplateREQ{
			TemplateName: "Bad Name!",
			TemplateBody: "正文",
		})
		var saved domain.PromptTemplateSaveRESP
		h.mustOK(env, code, "保存模板", &saved)
		if saved.TemplateName != "badname" {
			t.Fatalf("应回清洗后的名字 badname，实际 %q", saved.TemplateName)
		}

		env, code = h.call("GET", "/templates", nil)
		var list domain.TemplateListRESP
		h.mustOK(env, code, "模板清单", &list)
		found := false
		for _, row := range list.Rows {
			if row.TemplateName == saved.TemplateName {
				found = true
			}
		}
		if !found {
			t.Fatalf("清单里应有 %q，实际 %+v", saved.TemplateName, list.Rows)
		}

		// 用返回的名字删得掉；用原文删不掉（清洗不是往返的）。
		env, code = h.call("POST", "/templates/delete", domain.PromptTemplateREQ{TemplateName: saved.TemplateName})
		h.mustOK(env, code, "按返回名删除", nil)
		if pkg.Exists(filepath.Join(h.layout.TemplatesDir, "badname.md")) {
			t.Fatal("删除后文件不该还在")
		}
	})

	// 名字全非法时直接拒，不落一个空名文件。
	t.Run("模板名全非法即拒", func(t *testing.T) {
		h := newHarness(t)
		env, _ := h.call("POST", "/templates/save", domain.PromptTemplateREQ{
			TemplateName: "!!!",
			TemplateBody: "正文",
		})
		if env.Code != pkg.CodeBadRequest {
			t.Fatalf("非法模板名应回 %q，实际 %q（%s）", pkg.CodeBadRequest, env.Code, env.Message)
		}
	})

	// 路径守卫按真实路径校验：工作区里放一个指向外面的链接不能成为第二条出口。
	t.Run("路径守卫挡符号链接", func(t *testing.T) {
		h := newHarness(t)
		workspace := t.TempDir()
		outside := t.TempDir()
		secretPath := filepath.Join(outside, "secret.txt")
		writeFile(t, secretPath, "不该被读到")
		mustSetWorkspace(t, h, workspace)

		link := filepath.Join(workspace, "escape")
		if err := makeDirLink(link, outside); err != nil {
			t.Skipf("这台机器上建不了目录链接：%v", err)
		}

		env, _ := h.call("GET", "/files/read?path="+filepath.Join(link, "secret.txt"), nil)
		if env.Code != pkg.CodePathOutside {
			t.Fatalf("经符号链接读工作区外应回 %q，实际 %q（%s）", pkg.CodePathOutside, env.Code, env.Message)
		}
		if strings.Contains(string(env.Data), "不该被读到") {
			t.Fatal("越界内容不该出现在应答里")
		}

		// 写路径同理：不能借链接把文件写到工作区外。
		env, _ = h.call("POST", "/files/save", domain.FileSaveREQ{
			FilePath: filepath.Join(link, "planted.txt"),
			FileText: "被写进来了",
		})
		if env.Code != pkg.CodePathOutside {
			t.Fatalf("经符号链接写工作区外应回 %q，实际 %q", pkg.CodePathOutside, env.Code)
		}
		if pkg.Exists(filepath.Join(outside, "planted.txt")) {
			t.Fatal("工作区外不该出现被写入的文件")
		}
	})

	// 请求体上限：没有上限时一个请求就能把进程撑爆。
	t.Run("请求体超限即拒", func(t *testing.T) {
		h := newHarness(t)
		// 分多个键塞进去：单键超 16MB 也会被同一条上限挡住，
		// 这里明确构造「总量超限」而不是依赖某个业务字段的长度校验。
		payload := map[string]string{"ui.theme": strings.Repeat("x", 17<<20)}
		env, code := h.call("POST", "/settings/save", payload)
		if code != http.StatusOK {
			t.Fatalf("超限请求体也必须回信封（HTTP 200），实际 %d", code)
		}
		if env.Code == pkg.CodeOK {
			t.Fatal("超过上限的请求体应被拒")
		}
	})

	// SSE 队列溢出：慢客户端被摘掉，广播本身永远不被它拖住。
	// 被摘掉的连接靠前端重连 + since 补帧自愈，所以这里断言的是
	// 「广播不阻塞」「订户表不再挂着它」「健康订户的帧号严格递增」。
	t.Run("SSE 慢订户被摘掉", func(t *testing.T) {
		hub := sse.New(16)
		var mu sync.Mutex
		var seqs []int64
		healthyCancel, _ := hub.Subscribe(0, func(frame sse.Frame) error {
			mu.Lock()
			seqs = append(seqs, frame.Seq)
			mu.Unlock()
			return nil
		})
		defer healthyCancel()

		// 一个卡住的订户：队列填满之后必须被摘掉，而不是把广播堵住。
		release := make(chan struct{})
		defer close(release)
		var blocked atomic.Int64
		slowCancel, _ := hub.Subscribe(0, func(frame sse.Frame) error {
			blocked.Add(1)
			<-release
			return nil
		})
		defer slowCancel()

		// 节奏慢到健康订户跟得上（队列 64），而卡住的那个必然填满。
		const total = 200
		started := time.Now()
		for i := 0; i < total; i++ {
			hub.Broadcast("test:frame", map[string]any{"i": i})
			time.Sleep(time.Millisecond)
		}
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Fatalf("广播被慢订户堵住了：%d 帧耗时 %v", total, elapsed)
		}

		// 订户表里只剩健康那一个：溢出的订户必须销号，否则每帧都为它白跑一次入队。
		if got := hub.Subscribers(); got != 1 {
			t.Fatalf("慢订户应被摘掉，订户数应为 1，实际 %d", got)
		}

		// 健康订户的帧号严格递增：发号与入队同源。
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			got := len(seqs)
			mu.Unlock()
			if got >= total {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(seqs) != total {
			t.Fatalf("健康订户应收满 %d 帧，实际 %d", total, len(seqs))
		}
		for i := 1; i < len(seqs); i++ {
			if seqs[i] != seqs[i-1]+1 {
				t.Fatalf("健康订户的 seq 应连续：第 %d 帧是 %d，前一帧是 %d", i, seqs[i], seqs[i-1])
			}
		}
	})

	// 排队两条队的出参恒为数组：空队是 [] 而不是 null。
	// 界面把两队直接摊平进输入框，null 会在那里炸掉。
	t.Run("排队出参恒为数组", func(t *testing.T) {
		h := newHarness(t)
		env, code := h.call("POST", "/chat/stop", nil)
		if code != http.StatusOK || env.Code != pkg.CodeOK {
			t.Fatalf("中止应回成功，实际 http=%d code=%q", code, env.Code)
		}
		var raw map[string]any
		if err := json.Unmarshal(env.Data, &raw); err != nil {
			t.Fatalf("出参应为对象：%v", err)
		}
		for _, key := range []string{"steering", "follow_up"} {
			value, ok := raw[key]
			if !ok {
				t.Fatalf("出参应带 %s 字段：%v", key, raw)
			}
			if _, isArray := value.([]any); !isArray {
				t.Fatalf("%s 出参应是数组，实际 %T（%v）", key, value, value)
			}
		}
	})

	// 模板保存的出参形状：前端按信封解，新增的 template_name 必须是平铺字段。
	t.Run("模板出参形状", func(t *testing.T) {
		h := newHarness(t)
		env, code := h.call("POST", "/templates/save", domain.PromptTemplateREQ{
			TemplateName: "keep-me",
			TemplateBody: "正文",
		})
		if code != http.StatusOK || env.Code != pkg.CodeOK {
			t.Fatalf("保存模板应成功，实际 http=%d code=%q", code, env.Code)
		}
		var raw map[string]any
		if err := json.Unmarshal(env.Data, &raw); err != nil {
			t.Fatalf("出参应为对象：%v", err)
		}
		if _, ok := raw["ok"]; !ok {
			t.Fatalf("出参应仍带 ok 字段：%v", raw)
		}
		if name, _ := raw["template_name"].(string); name != "keep-me" {
			t.Fatalf("出参应带 template_name，实际 %v", raw)
		}
	})
}
