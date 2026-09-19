package tool_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/internal/tool"
	exectool "WorkBaby/internal/tool/exec"
	filetool "WorkBaby/internal/tool/file"
)

// mockWorkspaceResolver 模拟装配期 wsResolver：绑定目录表命中返回绑定目录，否则回落默认根。
func mockWorkspaceResolver(bound map[string]string, def string) tool.RootResolver {
	return func(sessionID string) string {
		if d, ok := bound[sessionID]; ok {
			return d
		}
		return def
	}
}

// TestWorkspaceLinkWriteExec 会话绑定本地目录后的工具落点链路：
//
//	file_write/file_read 相对路径 → 绑定目录；exec 缺省 cwd → 绑定目录。
//
// 回归点：曾因 exec 不联动工作区，Agent「选目录后跑命令」落在进程目录。
func TestWorkspaceLinkWriteExec(t *testing.T) {
	proj := t.TempDir() // 用户选择的本地文件夹
	def := filepath.Join(t.TempDir(), "default")
	if err := os.MkdirAll(def, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := mockWorkspaceResolver(map[string]string{"SESSION_x": proj}, def)
	ctx := tool.WithRunIdentity(context.Background(), "RUN_x", "SESSION_x")

	// 1) file_write 相对路径必须落在绑定目录，不得落到默认根
	w := filetool.NewWrite(resolver, def)
	if res := w.Execute(ctx, json.RawMessage(`{"path":"hello.md","content":"hi workbaby"}`)); res.Err != nil {
		t.Fatalf("file_write failed: %v", res.Err)
	}
	if _, err := os.Stat(filepath.Join(proj, "hello.md")); err != nil {
		t.Fatalf("written file should land in bound workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(def, "hello.md")); err == nil {
		t.Fatal("must NOT write to default root while a workspace is bound")
	}

	// 2) file_read 相对路径从绑定目录读
	r := filetool.NewRead(resolver, def)
	if res := r.Execute(ctx, json.RawMessage(`{"path":"hello.md"}`)); res.Err != nil {
		t.Fatalf("file_read failed: %v", res.Err)
	} else if !strings.Contains(res.Content, "hi workbaby") {
		t.Fatalf("read content mismatch: %q", res.Content)
	}

	// 3) exec 缺省 cwd 跟随绑定目录（cmd /c cd 打印当前目录验证）。
	//    依赖系统 shell 在 PATH 中；受限沙箱/精简环境（如 CI 容器）没有 cmd，跳过该子项。
	if _, err := exec.LookPath("cmd"); err != nil {
		t.Log("skip exec cwd assertion: cmd not found in PATH")
		return
	}
	policy := tool.DefaultExecPolicy()
	policy.AllowedBinaries = []string{"cmd"}
	ex := exectool.New(policy).WithRootResolver(tool.ResolveRoot(resolver, ""))
	if res := ex.Execute(ctx, json.RawMessage(`{"command":"cmd","args":["/c","cd"]}`)); res.Err != nil {
		t.Fatalf("exec failed: %v", res.Err)
	} else if !strings.Contains(res.Content, proj) {
		t.Fatalf("exec cwd should follow bound workspace, got output: %q", res.Content)
	}
}
