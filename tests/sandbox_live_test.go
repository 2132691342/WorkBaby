//go:build live

// 实时沙箱探针：验证「内置 node → 内置 npm → 装 pi → RPC 握手」这条链路。
// 只手动跑（go test ./tests -tags live -run TestSandboxChain -v），CI 与默认门禁不执行：
// 它要解压 115MB 运行时并联网装包。
package tests

import (
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/chat"
	"WorkBaby/backend/service/engine"
)

// TestSandboxChain 断言沙箱链路每一环真的通：运行时解压、npm 装 pi、RPC 取状态。
func TestSandboxChain(t *testing.T) {
	root, err := filepath.Abs(filepath.Join(".."))
	if err != nil {
		t.Fatalf("解析仓库根失败：%v", err)
	}
	layout := pkg.ResolveLayout(root, true)
	t.Logf("数据目录：%s", layout.DataDir)

	rt := engine.New(layout)
	problems := rt.ProvisionRuntime(func(phase, message string) {
		t.Logf("运行时 %s：%s", phase, message)
	})
	for _, problem := range problems {
		t.Logf("运行时问题：%s", problem)
	}
	if !layout.NodeReady() {
		t.Fatalf("内置 node 未就绪：%s", layout.NodeExe())
	}
	t.Logf("node=%s git=%s python=%s", rt.NodeVersion(), rt.GitVersion(), rt.PythonVersion())

	EnsurePi(t, rt)

	nodeExe, err := rt.NodeExe()
	if err != nil {
		t.Fatalf("取内置 node 失败：%v", err)
	}
	client := chat.NewPiRpc(nodeExe, rt.PiEntry(), layout.DataDir, layout.SandboxEnv(nil), chat.Handlers{
		OnEvent:     func(rec chat.Record) { t.Logf("事件：%v", rec["type"]) },
		OnUiRequest: func(rec chat.Record) { t.Logf("UI 请求：%v", rec["method"]) },
		OnExit:      func(code int) { t.Logf("引擎退出：%d", code) },
	})
	defer client.Stop()
	if err := client.Start(); err != nil {
		t.Fatalf("引擎启动失败：%v", err)
	}
	state, err := client.Request(chat.Record{"type": "get_state"}, 0)
	if err != nil {
		t.Fatalf("get_state 失败：%v", err)
	}
	fields, ok := state.(map[string]any)
	if !ok {
		t.Fatalf("get_state 应答不是对象：%#v", state)
	}
	if _, ok := fields["thinkingLevel"]; !ok {
		t.Fatalf("get_state 应答缺 thinkingLevel：%#v", fields)
	}
	cmds, err := client.Request(chat.Record{"type": "get_commands"}, 0)
	if err != nil {
		t.Fatalf("get_commands 失败：%v", err)
	}
	if list, ok := cmds.(map[string]any)["commands"].([]any); ok {
		t.Logf("斜杠命令 %d 条", len(list))
	}
	stats, err := client.Request(chat.Record{"type": "get_session_stats"}, 0)
	if err != nil {
		t.Fatalf("get_session_stats 失败：%v", err)
	}
	t.Logf("会话统计：%v", stats)
}

// EnsurePi 用内置 npm 装 pi 本体；已装同版本则跳过。
func EnsurePi(t *testing.T, rt *engine.Runtime) {
	t.Helper()
	if rt.PiInstalled() {
		t.Logf("pi 已装：%s", rt.PiVersion())
		return
	}
	layout := rt.Layout
	t.Logf("开始安装 pi 到 %s", layout.PiAppDir)
	res, err := rt.PiInstall(func(line string) { t.Logf("npm: %s", line) })
	if err != nil {
		if res != nil {
			t.Fatalf("pi 安装失败：%v\nstdout=%s\nstderr=%s", err, res.Stdout, res.Stderr)
		}
		t.Fatalf("pi 安装失败：%v", err)
	}
	t.Logf("pi 安装完成：%s", rt.PiVersion())
}
