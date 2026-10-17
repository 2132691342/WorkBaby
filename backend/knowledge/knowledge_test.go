// 知识库链路：加文档 → 建索引 → 检索 → 删除级联 → Office 抽取 → 重建索引后台任务。
// 单独测任一层都测不出检索失效：文档表、切片表与 FTS5 虚表是协作关系。
package knowledge

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/db"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

func newService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	gdb, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() {
		pool, _ := gdb.DB()
		_ = pool.Close()
	})
	return New(repo.New(gdb))
}

func addFile(t *testing.T, svc *Service, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := pkg.WriteText(path, body); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.Add([]string{path}); err != nil || n != 1 {
		t.Fatalf("添加文档失败: n=%d err=%v", n, err)
	}
	return path
}

// 一条链走完：加 → 查 → 删 → 再查 + Office 抽取 + 重建索引后台任务。
// 两字查询靠子串兜底；删除连带清切片；pptx 页序按文件序号排，不靠 zip 条目顺序。
func TestKnowledgeChain(t *testing.T) {
	t.Run("加文档、检索兜底、删除级联与 Office 页序", func(t *testing.T) {
		svc := newService(t)
		addFile(t, svc, "季度报告.md", "第三季度销售额增长了 18%，主要来自华东区的渠道拓展。\n客户满意度提升到 92%。")
		flowPath := addFile(t, svc, "流程笔记.txt", "预算审批流程需要三级签字")

		docs, err := svc.List()
		if err != nil || len(docs) != 2 {
			t.Fatalf("应有两个文档: %v", err)
		}
		var flowID string
		for _, d := range docs {
			if d.Status != domain.DocIndexed {
				t.Fatalf("文档应已建索引，实际 %s（路径 %s）", d.Status, d.Path)
			}
			if d.Path == flowPath {
				flowID = d.ID
			}
		}
		if flowID == "" {
			t.Fatal("列表里找不到刚加入的流程笔记")
		}

		hits, err := svc.Search(context.Background(), "季度销售额", 5)
		if err != nil || len(hits) == 0 {
			t.Fatalf("应能检索到内容: err=%v hits=%d", err, len(hits))
		}
		if !strings.Contains(hits[0].Title, "季度报告") || !strings.Contains(hits[0].Content, "销售额") {
			t.Fatalf("命中结果指向的文档或片段不对: %+v", hits[0])
		}
		if short, err := svc.Search(context.Background(), "预算", 5); err != nil || len(short) == 0 {
			t.Fatalf("两字查询应有子串兜底结果: err=%v hits=%d", err, len(short))
		} else if !strings.Contains(short[0].Content, "预算") {
			t.Fatalf("子串兜底命中的片段不对: %+v", short[0])
		}

		if err := svc.Delete(flowID); err != nil {
			t.Fatal(err)
		}
		if left, err := svc.List(); err != nil || len(left) != 1 {
			t.Fatalf("删除后应只剩一个文档: err=%v n=%d", err, len(left))
		}
		if gone, err := svc.Search(context.Background(), "三级签字", 5); err != nil || len(gone) != 0 {
			t.Fatalf("删除后不应还能检索到内容: err=%v hits=%d", err, len(gone))
		}

		// 刻意先写 slide2：抽取结果必须仍按页号排序。
		pptxPath := filepath.Join(t.TempDir(), "汇报.pptx")
		pf, err := os.Create(pptxPath)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(pf)
		slides := map[string]string{
			"ppt/slides/slide2.xml": `<a:p><a:t>第二页讲成本控制</a:t></a:p>`,
			"ppt/slides/slide1.xml": `<a:p><a:t>第一页讲季度增长</a:t></a:p>`,
		}
		for _, name := range []string{"ppt/slides/slide2.xml", "ppt/slides/slide1.xml"} {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte(slides[name])); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := pf.Close(); err != nil {
			t.Fatal(err)
		}
		if n, err := svc.Add([]string{pptxPath}); err != nil || n != 1 {
			t.Fatalf("添加 pptx 失败: n=%d err=%v", n, err)
		}
		text, err := LoadText(pptxPath)
		if err != nil {
			t.Fatalf("抽取 pptx 失败: %v", err)
		}
		if i1, i2 := strings.Index(text, "第一页"), strings.Index(text, "第二页"); i1 < 0 || i2 < 0 || i1 > i2 {
			t.Fatalf("页序不对或内容缺失: %q", text)
		}
		phits, err := svc.Search(context.Background(), "成本控制", 5)
		if err != nil || len(phits) == 0 {
			t.Fatalf("pptx 内容应可检索: err=%v hits=%d", err, len(phits))
		}
		if !strings.Contains(phits[0].Content, "成本控制") {
			t.Fatalf("pptx 命中的片段不对: %+v", phits[0])
		}
	})

	// 重建索引是后台任务：跑满进度、拒绝重入、结束后必须留下 finished_at。
	t.Run("重建索引后台任务", func(t *testing.T) {
		svc := newService(t)
		for i := 0; i < 3; i++ {
			addFile(t, svc, "doc.txt", "关于石板与靛蓝的配色说明，重复内容用于撑开切片。")
		}

		started, err := svc.StartReindex()
		if err != nil || !started {
			t.Fatalf("启动重建失败: started=%v err=%v", started, err)
		}
		// 重入会把两个任务的进度读成混合值，必须被拒。
		if again, err := svc.StartReindex(); err != nil || again {
			t.Fatalf("重复启动应被拒: again=%v err=%v", again, err)
		}

		st := svc.ReindexStatus()
		deadline := time.Now().Add(5 * time.Second)
		for st.Running && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			st = svc.ReindexStatus()
		}
		if st.Running {
			t.Fatal("重建任务没有在预期时间内结束")
		}
		if st.Total != 3 || st.Done+st.Failed != st.Total {
			t.Fatalf("进度没有跑满: done=%d failed=%d total=%d", st.Done, st.Failed, st.Total)
		}
		if st.FinishedAt == 0 {
			t.Fatal("结束时间未记录：前端只能靠它判断这一趟跑完了")
		}
		// 没任务在跑时取消要如实返回 false，不能假装成功。
		if svc.CancelReindex() {
			t.Fatal("没有任务在跑时取消应返回 false")
		}
	})

	// MCP 出口是助手唯一能看到知识库的通道：协议帧坏一行，症状就是「搜索没结果」。
	t.Run("MCP 出口：握手、工具声明与检索调用", func(t *testing.T) {
		svc := newService(t)
		addFile(t, svc, "报销制度.md", "差旅住宿标准：一线城市每晚 500 元，其他城市 350 元。")

		// 两根管道拼出一条 stdio：测试写 serverIn / 读 serverOut，server 反过来。
		serverIn, clientWrite := io.Pipe()
		clientRead, serverWrite := io.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = NewMCPServer(svc, serverIn, serverWrite).Serve()
		}()
		defer func() {
			_ = clientWrite.Close()
			<-done
			_ = clientRead.Close()
		}()

		sc := bufio.NewScanner(clientRead)
		write := func(payload string) {
			t.Helper()
			if _, err := io.WriteString(clientWrite, payload+"\n"); err != nil {
				t.Fatalf("写入请求失败: %v", err)
			}
		}
		// call 发一条请求并读回它那一条响应；通知只 write，不走这里。
		call := func(payload string) map[string]any {
			t.Helper()
			write(payload)
			if !sc.Scan() {
				t.Fatalf("没有读到响应帧: %s（%v）", payload, sc.Err())
			}
			var msg map[string]any
			if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
				t.Fatalf("响应不是合法 JSON: %v\n%s", err, sc.Text())
			}
			return msg
		}

		initResp := call(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
		result, _ := initResp["result"].(map[string]any)
		// 版本必须按客户端给的答：硬回自己的版本会让握手直接谈不拢。
		if result["protocolVersion"] != "2025-06-18" {
			t.Fatalf("协议版本没有回客户端带来的值: %v", initResp)
		}

		// 通知没有 id：回了就会错位在途请求的响应配对上。
		write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		listResp := call(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
		tools, _ := listResp["result"].(map[string]any)["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("应当只声明一个工具: %v", listResp)
		}
		tool := tools[0].(map[string]any)
		if tool["name"] != mcpToolSearch {
			t.Fatalf("工具名不对: %v", tool["name"])
		}

		callResp := call(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"knowledge_search","arguments":{"query":"住宿标准","limit":3}}}`)
		calls, _ := callResp["result"].(map[string]any)["content"].([]any)
		if len(calls) != 1 {
			t.Fatalf("检索响应缺少内容块: %v", callResp)
		}
		text := calls[0].(map[string]any)["text"].(string)
		if !strings.Contains(text, "500") || !strings.Contains(text, "报销制度") {
			t.Fatalf("检索结果没有带上命中的文档与正文: %q", text)
		}

		empty := call(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"knowledge_search","arguments":{"query":"火星殖民配额"}}}`)
		emptyText := empty["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
		// 零命中要给一句话：空串会让模型以为工具坏了，转而凭记忆编。
		if !strings.Contains(emptyText, "没有相关内容") {
			t.Fatalf("零命中应当明说: %q", emptyText)
		}

		bad := call(`{"jsonrpc":"2.0","id":5,"method":"unknown/method"}`)
		if bad["error"] == nil {
			t.Fatalf("未知方法必须报错而不是静默: %v", bad)
		}
	})
}
