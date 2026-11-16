// 模型的眼睛：工作区预索引（本地）与联网（外部）。
// 两条链路的输入都不可信——路径来自对话内容、地址也是——所以边界一律在工具层与接口层各挡一遍；
// 索引的价值在大仓库：找文件从「几秒的全盘 grep」变成「毫秒级查表」。
package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/index"
	"WorkBaby/backend/web"
)

func TestWorkspace(t *testing.T) {
	// 索引收录边界：源文件与符号进，二进制与依赖目录不进。
	t.Run("索引收录边界", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "main.go"), `package main

func StartServer() error {
	return nil
}

type Config struct {
	Port int
}
`)
		writeFile(t, filepath.Join(root, "app.ts"), `export function renderApp() {}
export class Widget {}
`)
		// 二进制与依赖目录不该进索引：node_modules 进了索引就是一堆噪声命中。
		writeFile(t, filepath.Join(root, "logo.png"), "\x89PNG\r\n\x1a\n")
		noise := filepath.Join(root, "node_modules", "pkg")
		if err := os.MkdirAll(noise, 0o755); err != nil {
			t.Fatalf("建目录失败：%v", err)
		}
		writeFile(t, filepath.Join(noise, "index.js"), "module.exports = {}")

		docs, err := index.Build(context.Background(), root)
		if err != nil {
			t.Fatalf("建索引失败：%v", err)
		}
		paths := map[string]bool{}
		symbols := map[string]bool{}
		for _, doc := range docs {
			paths[doc.DocPath] = true
			if doc.DocKind == index.KindSymbol {
				symbols[doc.DocSymbol] = true
			}
			if strings.Contains(doc.DocPath, "node_modules") {
				t.Fatalf("依赖目录不该进索引：%s", doc.DocPath)
			}
		}
		if !paths["main.go"] || !paths["app.ts"] {
			t.Fatalf("源文件该进索引：%v", paths)
		}
		if paths["logo.png"] {
			t.Fatal("二进制不该进索引")
		}
		if !symbols["StartServer"] || !symbols["Config"] {
			t.Fatalf("Go 符号该被抽出：%v", symbols)
		}
		if !symbols["renderApp"] || !symbols["Widget"] {
			t.Fatalf("TS 符号该被抽出：%v", symbols)
		}
	})

	// 索引排序：符号名命中排在路径命中之前，常用近用的再往前。
	t.Run("索引排序", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "helper.go"), `package helper

func ParseConfig() {}
`)
		writeFile(t, filepath.Join(root, "parse-config-notes.md"), "notes")

		docs, err := index.Build(context.Background(), root)
		if err != nil {
			t.Fatalf("建索引失败：%v", err)
		}
		hits := index.Search(docs, "parseconfig", nil, 10)
		if len(hits) == 0 {
			t.Fatal("该搜到 ParseConfig 符号")
		}
		if hits[0].HitDoc.DocKind != index.KindSymbol {
			t.Fatalf("符号命中该排前面，实际是 %s", hits[0].HitDoc.DocKind)
		}
		if hits[0].HitDoc.DocSymbol != "ParseConfig" {
			t.Fatalf("该命中 ParseConfig：%v", hits[0].HitDoc)
		}

		// ---- frecency：常用且近用的文档排得更前 ----
		ranked := []index.Doc{
			{DocID: "F:a.go", DocPath: "a.go", DocKind: index.KindFile},
			{DocID: "F:b.go", DocPath: "b.go", DocKind: index.KindFile},
		}
		frecency := index.NewFrecency()
		// b.go 被反复用到，a.go 从没用过。
		for range 5 {
			frecency.Record("F:b.go", 1)
		}
		frecency.Record("F:a.go", 1)

		hits = index.Search(ranked, "go", frecency, 10)
		if len(hits) < 2 {
			t.Fatalf("该搜到两个文件，实际 %d", len(hits))
		}
		if hits[0].HitDoc.DocPath != "b.go" {
			t.Fatalf("常用文件该排前面，实际 %s", hits[0].HitDoc.DocPath)
		}
	})

	// 联网地址拦截：内网与本机地址在工具层被拦下，公网地址放行。
	// 模型给的地址不能成为读到本机服务的通道。
	t.Run("联网地址拦截", func(t *testing.T) {
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
	})

	// 正文抽取：内容留下、脚本与导航去掉、HTML 实体还原、
	// 跳转链接还原成真实地址——都是「喂给模型的原料要干净」这一件事。
	t.Run("正文抽取", func(t *testing.T) {
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
	})

	// 联网接口链路：路由接上了，且接口层也挡住内网地址——
	// 路由里的方法名拼错编译期查不出来（同包未导出方法），只有真打一次才知道。
	t.Run("联网接口链路", func(t *testing.T) {
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
	})
}
