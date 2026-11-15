// 假 MCP 服务器：测试二进制自起自己——子进程带 --wb-mcp-server 时进服务器模式，
// stdin 收 JSON-RPC、stdout 回一行一个应答；不拉真进程、不联网。
package tests

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// TestMain 在 --wb-mcp-server 下切入服务器模式：测试二进制自起自己时，
// 子进程跑的仍是这套 testmain，不在这里拦下就会去跑整轮测试并挂死。
func TestMain(m *testing.M) {
	for _, arg := range os.Args[1:] {
		if arg == "--wb-mcp-server" {
			runFakeMcpServer(os.Stdin, os.Stdout)
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

// mcpFakeTool 是假服务器提供的工具。
type mcpFakeTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// fakeMcpTools 假服务器的工具清单。
func fakeMcpTools() []mcpFakeTool {
	return []mcpFakeTool{
		{
			Name:        "echo",
			Description: "把输入原样说回来，测试用",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "要说的话"},
				},
				"required": []string{"text"},
			},
		},
		{
			Name:        "fail_always",
			Description: "永远返回 isError 的工具，测试错误通道",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

// runFakeMcpServer 假服务器主循环：stdin 收 JSON-RPC，stdout 回一行一个应答。
func runFakeMcpServer(stdin io.Reader, stdout io.Writer) {
	reader := bufio.NewReader(stdin)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		if line != "" {
			respondFakeMcp(line, stdout)
		}
		if err != nil {
			return
		}
	}
}

// respondFakeMcp 处理一条请求。
func respondFakeMcp(line string, stdout io.Writer) {
	var req struct {
		Jsonrpc string         `json:"jsonrpc"`
		Id      json.Number    `json:"id"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params"`
	}
	if json.Unmarshal([]byte(line), &req) != nil {
		return
	}
	switch req.Method {
	case "initialize":
		writeFakeResult(stdout, req.Id, map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "wb-fake-mcp", "version": "0.1.0"},
		})
	case "tools/list":
		writeFakeResult(stdout, req.Id, map[string]any{"tools": fakeMcpTools()})
	case "tools/call":
		name, _ := req.Params["name"].(string)
		if name == "fail_always" {
			writeFakeResult(stdout, req.Id, map[string]any{
				"isError": true,
				"content": []map[string]any{{"type": "text", "text": "这个工具就是用来失败的"}},
			})
			return
		}
		if name == "echo" {
			args, _ := req.Params["arguments"].(map[string]any)
			text, _ := args["text"].(string)
			writeFakeResult(stdout, req.Id, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "echo: " + text}},
			})
			return
		}
		writeFakeError(stdout, req.Id, -32602, "未知工具："+name)
	default:
		if req.Id != "" {
			writeFakeError(stdout, req.Id, -32601, "未知方法："+req.Method)
		}
	}
}

// writeFakeResult 回一个成功应答。
func writeFakeResult(stdout io.Writer, id json.Number, result map[string]any) {
	writeFakeLine(stdout, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

// writeFakeError 回一个 JSON-RPC 错误。
func writeFakeError(stdout io.Writer, id json.Number, code int, message string) {
	writeFakeLine(stdout, map[string]any{"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": message}})
}

// writeFakeLine 一行一个 JSON。
func writeFakeLine(stdout io.Writer, rec map[string]any) {
	raw, err := json.Marshal(rec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake server marshal:", err)
		return
	}
	fmt.Fprintln(stdout, string(raw))
}
