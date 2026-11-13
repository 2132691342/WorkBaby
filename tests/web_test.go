// 联网能力：地址安全、正文抽取与接口链路。
// 这条能力的安全边界在地址校验上——模型给的地址来自对话内容，
// 不能让它读到本机服务，所以 SSRF 的拦截必须可靠，且接口层与工具层都要挡。
package tests

import (
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/web"
)

// TestWebAddressGuard 断言内网与本机地址在工具层被拦下。
func TestWebAddressGuard(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1:8080/admin",
		"http://localhost/",
		"https://[::1]/",
		"http://10.0.0.5/",
		"http://192.168.1.1/",
		"http://172.16.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.64.0.1/",
		"file:///C:/Windows/win.ini",
		"ftp://example.com/",
		"",
	}
	for _, url := range blocked {
		if err := web.ValidateURL(url); err == nil {
			t.Fatalf("这个地址该被拦下：%q", url)
		}
	}

	allowed := []string{
		"https://example.com",
		"http://example.com/path?q=1",
		"https://sub.domain.co.uk/a/b",
	}
	for _, url := range allowed {
		if err := web.ValidateURL(url); err != nil {
			t.Fatalf("这个地址不该被拦：%q（%v）", url, err)
		}
	}
}

// TestWebExtractReadable 断言正文抽取与地址还原：内容留下、脚本与导航去掉、
// HTML 实体还原、跳转链接还原成真实地址——都是「喂给模型的原料要干净」这一件事。
func TestWebExtractReadable(t *testing.T) {
	raw := `<html><head><title>标题</title><style>body{color:red}</style></head>
	<body>
		<script>alert("xss")</script>
		<nav><a href="/home">首页</a></nav>
		<h1>主标题</h1>
		<p>这是<strong>正文</strong>第一段。</p>
		<ul><li>条目一</li><li>条目二</li></ul>
	</body></html>`

	text := web.ExtractReadable(raw)
	for _, want := range []string{"主标题", "正文", "条目一"} {
		if !strings.Contains(text, want) {
			t.Fatalf("该留下的内容丢了 %q：\n%s", want, text)
		}
	}
	// 脚本、样式与导航是噪声，留着会占上下文还会误导模型。
	for _, noise := range []string{"alert", "color:red", "首页"} {
		if strings.Contains(text, noise) {
			t.Fatalf("不该留下的噪声还在 %q：\n%s", noise, text)
		}
	}

	entities := web.ExtractReadable(`<html><body><p>A &amp; B &lt;tag&gt; &quot;quoted&quot;</p></body></html>`)
	if !strings.Contains(entities, "A & B") {
		t.Fatalf("实体该被还原：\n%s", entities)
	}
	if strings.Contains(entities, "&amp;") {
		t.Fatalf("实体不该原样留着：\n%s", entities)
	}

	// ---- 跳转链接还原：免密钥搜索走 HTML 端点，给的是追踪链接，
	// 不还原就等于把追踪地址喂给模型 ----
	rawURL := "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage&rut=abc"
	if got := web.NormalizeDuckURL(rawURL); got != "https://example.com/page" {
		t.Fatalf("跳转链接没还原：%q", got)
	}
	if web.NormalizeDuckURL("") != "" {
		t.Fatal("空链接该给空串")
	}
}

// TestWebEndpoints 断言联网接口链路接上了，且接口层也挡住内网地址。
// 路由里的方法名拼错编译期查不出来（那是同包的未导出方法），只有真打一次才知道。
func TestWebEndpoints(t *testing.T) {
	h := newHarness(t)

	env, code := h.call("GET", "/web/status", nil)
	var status domain.WebStatusRESP
	h.mustOK(env, code, "读联网状态", &status)
	if status.WebKind == "" {
		t.Fatal("联网状态该报出搜索服务种类")
	}
	if len(status.WebKinds) == 0 {
		t.Fatal("该列出可选的搜索服务")
	}

	// 保存后要能从存储读回来：状态要由设置派生，不能靠内存里的临时值。
	env, code = h.call("POST", "/web/settings", domain.WebSettingsREQ{
		WebKind:   "brave",
		WebApiKey: "test-key-123",
	})
	h.mustOK(env, code, "保存联网设置", &status)
	if status.WebKind != "brave" {
		t.Fatalf("搜索服务种类该存下来：%q", status.WebKind)
	}
	if !status.WebHasKey {
		t.Fatal("该记住密钥已配置")
	}
	env, code = h.call("GET", "/web/status", nil)
	h.mustOK(env, code, "复读联网状态", &status)
	if status.WebKind != "brave" {
		t.Fatalf("设置该被持久化：%q", status.WebKind)
	}

	// 不认识的搜索服务该被拒。
	env, _ = h.call("POST", "/web/settings", domain.WebSettingsREQ{WebKind: "not-a-provider"})
	if env.Code == "0" {
		t.Fatal("不认识的搜索服务该被拒")
	}

	// 内网地址在接口层就要被挡下：模型给的地址来自对话内容。
	env, _ = h.call("POST", "/web/fetch", domain.WebFetchREQ{FetchUrl: "http://127.0.0.1:9/secret"})
	if env.Code == "0" {
		t.Fatal("内网地址该被接口拦下")
	}
}
