package service

// MCP 连接测试：完成一次 JSON-RPC initialize 握手。
// stdio 拉真进程、http 发真请求——只测「进程存在」不测协议等于没测。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// mcpProbeTimeout 是握手总预算；条目自带的 timeout（毫秒）可以覆盖它。
const mcpProbeTimeout = 10 * time.Second

// Probe 测一个 MCP 服务能不能连通。
func (m *Mcp) Probe(serverName string) (domain.McpProbeRESP, error) {
	name := strings.TrimSpace(serverName)
	if name == "" {
		return domain.McpProbeRESP{}, pkg.Err(pkg.CodeMcpWrite, "服务名不能为空")
	}
	entry, ok := m.findEntry(name)
	if !ok {
		return domain.McpProbeRESP{}, pkg.Err(pkg.CodeMcpWrite, "未找到 MCP 服务："+name)
	}
	if entry["enabled"] == false {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: "服务已停用，先启用再测"}, nil
	}
	if url := stringField(entry, "url"); url != "" {
		return probeHttp(entry, url)
	}
	return m.probeStdio(entry)
}

// mcpCall 组一条 JSON-RPC 请求。
func mcpCall(id int, method string, params map[string]any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
}

// mcpInitializeParams 是握手参数：报出自己是壳，协议版本给主流的最新。
func mcpInitializeParams() map[string]any {
	return map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "WorkBaby", "version": "1.0"},
	}
}

// mcpTimeoutOf 条目自带的超时（pi 的 mcp.json 里单位是秒）；没配用缺省。
func mcpTimeoutOf(entry map[string]any, fallback time.Duration) time.Duration {
	if n, ok := entry["timeout"].(float64); ok && n > 0 {
		return time.Duration(n) * time.Second
	}
	return fallback
}

// mcpServerLabel 从握手应答里取「谁在应答」。
func mcpServerLabel(result map[string]any) string {
	info, _ := result["serverInfo"].(map[string]any)
	name := stringField(info, "name")
	version := stringField(info, "version")
	switch {
	case name != "" && version != "":
		return name + " " + version
	case name != "":
		return name
	default:
		return "未知服务"
	}
}

// probeHttp 对 streamable http 服务发 initialize 与 tools/list。
func probeHttp(entry map[string]any, url string) (domain.McpProbeRESP, error) {
	client := &http.Client{Timeout: mcpTimeoutOf(entry, mcpProbeTimeout)}
	res, body, err := mcpHttpPost(client, url, mcpCall(1, "initialize", mcpInitializeParams()), "")
	if err != nil {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: "连不上：" + mcpShort(err.Error())}, nil
	}
	result, errMsg := mcpResultOf(body)
	if errMsg != "" {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: errMsg}, nil
	}
	// streamable http 用会话头串后续请求；先补规范要求的 initialized 通知。
	session := res.Header.Get("Mcp-Session-Id")
	_, _, _ = mcpHttpPost(client, url, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}, session)
	_, listBody, err := mcpHttpPost(client, url, mcpCall(2, "tools/list", map[string]any{}), session)
	if err != nil {
		return domain.McpProbeRESP{ProbeOk: true, ProbeMessage: mcpServerLabel(result)}, nil
	}
	return domain.McpProbeRESP{
		ProbeOk:      true,
		ProbeMessage: mcpServerLabel(result),
		ProbeTools:   mcpToolCount(listBody),
	}, nil
}

// mcpHttpPost 发一条 JSON-RPC 并取回正文；应答可能是 JSON 也可能是 SSE 帧。
func mcpHttpPost(client *http.Client, url string, call map[string]any, session string) (*http.Response, []byte, error) {
	payload, err := json.Marshal(call)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return res, nil, err
	}
	return res, raw, nil
}

// mcpResultOf 从应答正文里取 result；error 或解析不了都给一句人话。
func mcpResultOf(raw []byte) (map[string]any, string) {
	rec := mcpFirstJSON(raw)
	if rec == nil {
		return nil, "应答不是 JSON-RPC：服务在但协议不对"
	}
	if errObj, ok := rec["error"].(map[string]any); ok {
		return nil, "握手被拒：" + stringField(errObj, "message")
	}
	result, _ := rec["result"].(map[string]any)
	if result == nil {
		return nil, "应答缺 result 字段"
	}
	return result, ""
}

// mcpFirstJSON 应答可能是整块 JSON，也可能裹在 SSE 的 data: 行里；两种都试。
func mcpFirstJSON(raw []byte) map[string]any {
	var whole map[string]any
	if pkg.UnmarshalJSON(raw, &whole) == nil {
		return whole
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var rec map[string]any
		if pkg.UnmarshalJSON([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &rec) == nil {
			return rec
		}
	}
	return nil
}

// mcpToolCount 从 tools/list 应答里数工具；数不出给 0。
func mcpToolCount(raw []byte) int {
	result, _ := mcpResultOf(raw)
	if result == nil {
		return 0
	}
	tools, _ := result["tools"].([]any)
	return len(tools)
}

// mcpShort 把网络错误原话压到一行可读长度。
func mcpShort(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	return pkg.Truncate(text, 200)
}

// probeStdio 拉起服务进程，走 stdin/stdout 完成 initialize 与 tools/list。
func (m *Mcp) probeStdio(entry map[string]any) (domain.McpProbeRESP, error) {
	command := stringField(entry, "command")
	if command == "" {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: "没有配置命令"}, nil
	}
	args := asStringSlice(entry["args"])
	cmd := exec.Command(command, args...)
	cmd.SysProcAttr = pkg.HiddenProcAttr()
	if dir := stringField(entry, "cwd"); pkg.Exists(dir) {
		cmd.Dir = dir
	}
	env := m.Layout.SandboxEnv(nil)
	if userEnv, ok := entry["env"].(map[string]any); ok {
		for key, value := range userEnv {
			env = append(env, key+"="+fmt.Sprint(value))
		}
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: "stdin 打开失败"}, nil
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: "stdout 打开失败"}, nil
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: "进程起不来：" + mcpShort(err.Error())}, nil
	}
	// 拿到结论就收摊：测试进程不需要优雅退出，杀树防 npx 类包装器留孤儿。
	pid := cmd.Process.Pid
	defer pkg.KillTree(pid)

	deadline := time.Now().Add(mcpTimeoutOf(entry, mcpProbeTimeout))
	write := func(rec map[string]any) bool {
		payload, err := json.Marshal(rec)
		if err != nil {
			return false
		}
		_, err = io.WriteString(stdin, string(payload)+"\n")
		return err == nil
	}
	lines := mcpLineChan(stdout)

	if !write(mcpCall(1, "initialize", mcpInitializeParams())) {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: "握手请求发不进去"}, nil
	}
	rec := mcpWaitFor(lines, 1, time.Until(deadline))
	result, errMsg := mcpResultOfRecord(rec)
	if errMsg != "" {
		return domain.McpProbeRESP{ProbeOk: false, ProbeMessage: errMsg}, nil
	}
	_ = write(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if !write(mcpCall(2, "tools/list", map[string]any{})) {
		return domain.McpProbeRESP{ProbeOk: true, ProbeMessage: mcpServerLabel(result)}, nil
	}
	listRec := mcpWaitFor(lines, 2, time.Until(deadline))
	return domain.McpProbeRESP{
		ProbeOk:      true,
		ProbeMessage: mcpServerLabel(result),
		ProbeTools:   mcpToolCountOfRecord(listRec),
	}, nil
}

// mcpLineChan 把 stdout 按 LF 切成行推进通道；读尽或出错即关闭。
func mcpLineChan(reader io.Reader) <-chan string {
	lines := make(chan string, 32)
	go func() {
		defer close(lines)
		buf := make([]byte, 32*1024)
		rest := ""
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				rest += string(buf[:n])
				for {
					idx := strings.IndexByte(rest, '\n')
					if idx < 0 {
						break
					}
					lines <- strings.TrimSuffix(rest[:idx], "\r")
					rest = rest[idx+1:]
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}

// mcpWaitFor 在预算内等指定 id 的应答；等不到给 nil。
func mcpWaitFor(lines <-chan string, id int, budget time.Duration) map[string]any {
	if budget <= 0 {
		return nil
	}
	deadline := time.After(budget)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			var rec map[string]any
			if pkg.UnmarshalJSON([]byte(line), &rec) != nil {
				continue
			}
			if got, _ := rec["id"].(float64); int(got) == id {
				return rec
			}
		case <-deadline:
			return nil
		}
	}
}

// mcpResultOfRecord 与 mcpResultOf 同义，吃的是已解析的记录。
func mcpResultOfRecord(rec map[string]any) (map[string]any, string) {
	if rec == nil {
		return nil, "握手超时：进程在但没应答"
	}
	if errObj, ok := rec["error"].(map[string]any); ok {
		return nil, "握手被拒：" + stringField(errObj, "message")
	}
	result, _ := rec["result"].(map[string]any)
	if result == nil {
		return nil, "应答缺 result 字段"
	}
	return result, ""
}

// mcpToolCountOfRecord 从 tools/list 记录里数工具。
func mcpToolCountOfRecord(rec map[string]any) int {
	result, _ := mcpResultOfRecord(rec)
	if result == nil {
		return 0
	}
	tools, _ := result["tools"].([]any)
	return len(tools)
}
