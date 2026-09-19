package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/internal/tool"
)

// mustJSON 入参序列化；测试内构造工具参数的统一入口。
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	bs, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bs
}

// guard_test.go 覆盖文件工具的两道「少犯错」护栏：
//   - .gitignore 感知：检索不把构建产物/日志当源码翻（噪声直接淹没有效命中）
//   - 写前必须读：file_edit 的 old_string 与磁盘不一致时「匹配到别处」会改坏文件

// TestGrepRespectsGitignore 命中 .gitignore 的文件与目录必须从检索结果里消失。
func TestGrepRespectsGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", rel, err)
		}
	}
	writeFile(".gitignore", "*.log\ndist/\n!keep.log\n")
	writeFile("src/keep.go", "NEEDLE in source")
	writeFile("src/keep.log", "NEEDLE in negated log")
	writeFile("debug.log", "NEEDLE in log")
	writeFile("dist/app.js", "NEEDLE in build output")

	res := NewGrep(nil, root).Execute(context.Background(), mustJSON(t, grepReq{Pattern: "NEEDLE", MaxResults: 50}))
	if res.Err != nil {
		t.Fatalf("grep: %v", res.Err)
	}
	if !strings.Contains(res.Content, "src/keep.go") {
		t.Fatalf("正常源码必须命中: %q", res.Content)
	}
	if !strings.Contains(res.Content, "src/keep.log") {
		t.Fatalf("! 反向规则应放行: %q", res.Content)
	}
	if strings.Contains(res.Content, "debug.log") {
		t.Fatalf("*.log 应被忽略: %q", res.Content)
	}
	if strings.Contains(res.Content, "dist/") {
		t.Fatalf("dist/ 整棵目录应被忽略: %q", res.Content)
	}
}

// TestEditRequiresReadFirst 无 file_read 直接 file_edit 必须被拒；读一次后放行。
func TestEditRequiresReadFirst(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx := tool.WithRunIdentity(context.Background(), "RUN_READ_FIRST", "SESSION_READ_FIRST")
	edit := NewEdit(nil, root)

	res := edit.Execute(ctx, mustJSON(t, editReq{Path: p, OldString: "hello", NewString: "hi"}))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "file_read") {
		t.Fatalf("未读先改必须被拒并指明原因, got %v", res.Err)
	}

	if readRes := NewRead(nil, root).Execute(ctx, mustJSON(t, readReq{Path: p})); readRes.Err != nil {
		t.Fatalf("read: %v", readRes.Err)
	}
	res = edit.Execute(ctx, mustJSON(t, editReq{Path: p, OldString: "hello", NewString: "hi"}))
	if res.Err != nil {
		t.Fatalf("读后编辑应放行: %v", res.Err)
	}
	bs, _ := os.ReadFile(p)
	if string(bs) != "hi" {
		t.Fatalf("content = %q", string(bs))
	}

	// 换一轮 run：读态不继承（文件可能已被外部改动）
	nextCtx := tool.WithRunIdentity(context.Background(), "RUN_READ_FIRST_2", "SESSION_READ_FIRST")
	res = edit.Execute(nextCtx, mustJSON(t, editReq{Path: p, OldString: "hi", NewString: "yo"}))
	if res.Err == nil {
		t.Fatalf("跨 run 必须重新读取后再编辑")
	}
}
