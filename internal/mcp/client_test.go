package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// fixtureEnv 让测试二进制以「MCP fixture server」模式重新执行自身，
// 免去测试期额外编译子程序的依赖（Go 官方推荐的 re-exec fixture 模式）。
const fixtureEnv = "WB_MCP_FIXTURE"

// fixtureMode 各场景对应的 fixture 行为。
const (
	fixtureOK    = "ok"
	fixtureNoise = "noise" // 先吐非 JSON 行再正常服务
	fixtureCrash = "crash" // 握手后 300ms 自杀
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fixtureEnv); mode != "" {
		runFixtureServer(mode)
		return
	}
	os.Exit(m.Run())
}

// dialFixture 以 fixture 模式拉起测试二进制自身作为 MCP server。
func dialFixture(t *testing.T, mode string) *StdioClient {
	t.Helper()
	c, err := DialStdio("fixture", os.Args[0], []string{"-test.run=^$"}, []string{fixtureEnv + "=" + mode}, nil)
	if err != nil {
		t.Fatalf("dial fixture: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestStdioClientHandshakeAndListTools(t *testing.T) {
	c := dialFixture(t, fixtureOK)
	ctx := context.Background()

	info, err := c.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if info.Name != "fixture" {
		t.Fatalf("server name = %q, want fixture", info.Name)
	}

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "echo" || tools[1].Name != "boom" {
		t.Fatalf("tools = %+v, want [echo boom]", tools)
	}
	if !json.Valid(tools[0].InputSchema) {
		t.Fatalf("echo schema is not valid json: %s", tools[0].InputSchema)
	}
}

// TestMCPAdapterExecute 走业务实际入口（Adapter）：成功路径回填内容、isError 路径
// 既回填原文又给出错误。Client 层的 CallTool 原样返回由此间接覆盖，不再单独设用例。
func TestMCPAdapterExecute(t *testing.T) {
	c := dialFixture(t, fixtureOK)
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	echo := &MCPAdapter{ServerName: "fixture", Client: c, Tool: ToolDef{Name: "echo", Description: "echo back"}}
	out := echo.Execute(ctx, json.RawMessage(`{"msg":"hi"}`))
	if out.Err != nil {
		t.Fatalf("execute echo: %v", out.Err)
	}
	if out.Content != "echo: hi" {
		t.Fatalf("content = %q, want %q", out.Content, "echo: hi")
	}

	boom := &MCPAdapter{ServerName: "fixture", Client: c, Tool: ToolDef{Name: "boom"}}
	out = boom.Execute(ctx, json.RawMessage(`{}`))
	if out.Err == nil {
		t.Fatalf("boom should surface an error")
	}
	if out.Content != "kaboom" { // isError 的内容仍要回填给 LLM
		t.Fatalf("content = %q, want %q", out.Content, "kaboom")
	}
}

func TestStdioClientDeadServerFailsCalls(t *testing.T) {
	c := dialFixture(t, fixtureCrash)
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	time.Sleep(600 * time.Millisecond) // 等 fixture 自杀完成

	_, err := c.ListTools(context.Background())
	if err == nil {
		t.Fatalf("list tools on dead server should fail")
	}
	ae, ok := pkg.As(err)
	if !ok {
		t.Fatalf("error should be AppError, got %T: %v", err, err)
	}
	if ae.Code != 8003 {
		t.Fatalf("code = %d, want 8003 (%v)", ae.Code, err)
	}
}

func TestManagerReloadRegistersAndUnregisters(t *testing.T) {
	toolReg := tool.NewRegistry()
	mgr := NewManager(toolReg)
	defer mgr.Close()

	srv := domain.McpServerDO{
		Name:      "fixture",
		Transport: domain.McpTransportStdio,
		Command:   os.Args[0],
		Args:      `["-test.run=^$"]`,
		Env:       `{"WB_MCP_FIXTURE":"ok"}`,
		Enabled:   true,
	}
	mgr.Reload(context.Background(), []domain.McpServerDO{srv})

	st := mgr.Status("fixture")
	if !st.Ready || st.ToolCount != 2 {
		t.Fatalf("status = %+v, want ready with 2 tools", st)
	}
	for _, name := range []string{"mcp_fixture_echo", "mcp_fixture_boom"} {
		if _, ok := toolReg.Get(name); !ok {
			t.Fatalf("tool %s should be registered", name)
		}
	}

	// 停用 → 子进程停止 + 工具注销
	srv.Enabled = false
	mgr.Reload(context.Background(), []domain.McpServerDO{srv})
	if _, ok := toolReg.Get("mcp_fixture_echo"); ok {
		t.Fatalf("tool should be unregistered after disabling server")
	}
	if st := mgr.Status("fixture"); st.Ready {
		t.Fatalf("status should not be ready after disable: %+v", st)
	}
}

func TestManagerStartupFailureDegrades(t *testing.T) {
	toolReg := tool.NewRegistry()
	mgr := NewManager(toolReg)
	defer mgr.Close()

	srv := domain.McpServerDO{
		Name: "broken", Transport: domain.McpTransportStdio,
		Command: "workbaby-definitely-not-a-command.exe", Enabled: true,
	}
	mgr.Reload(context.Background(), []domain.McpServerDO{srv})

	st := mgr.Status("broken")
	if st.Ready {
		t.Fatalf("broken server should not be ready")
	}
	if st.Error == "" {
		t.Fatalf("broken server should carry an error message")
	}
	if len(mgr.List()) != 1 {
		t.Fatalf("status list = %+v, want 1 entry", mgr.List())
	}
}

// ---- fixture server 实现 ----

func runFixtureServer(mode string) {
	if mode == fixtureNoise {
		_, _ = os.Stdout.WriteString("server warming up...\n")
	}
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		fixtureHandle(enc, mode, req.ID, req.Method, req.Params)
	}
}

func fixtureHandle(enc *json.Encoder, mode string, id *int64, method string, params json.RawMessage) {
	switch method {
	case "initialize":
		writeFixtureResp(enc, id, map[string]any{
			"protocolVersion": protocolVersion,
			"serverInfo":      map[string]any{"name": "fixture", "version": "1.0.0"},
		}, nil)
		if mode == fixtureCrash {
			go func() {
				time.Sleep(300 * time.Millisecond)
				os.Exit(1)
			}()
		}
	case "tools/list":
		writeFixtureResp(enc, id, map[string]any{"tools": []any{
			map[string]any{
				"name":        "echo",
				"description": "echo back",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"msg": map[string]any{"type": "string"}}, "required": []string{"msg"}},
			},
			map[string]any{"name": "boom", "description": "always fails", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
		}}, nil)
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(params, &p)
		switch p.Name {
		case "echo":
			msg, _ := p.Arguments["msg"].(string)
			writeFixtureResp(enc, id, map[string]any{"content": []any{map[string]any{"type": "text", "text": "echo: " + msg}}}, nil)
		case "boom":
			writeFixtureResp(enc, id, map[string]any{"content": []any{map[string]any{"type": "text", "text": "kaboom"}}, "isError": true}, nil)
		default:
			writeFixtureResp(enc, id, nil, map[string]any{"code": -32601, "message": "Method not found"})
		}
	default:
		if id != nil { // 通知（无 id）不响应
			writeFixtureResp(enc, id, nil, map[string]any{"code": -32601, "message": "Method not found"})
		}
	}
}

func writeFixtureResp(enc *json.Encoder, id *int64, result any, rpcErr any) {
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result, "error": rpcErr})
}
