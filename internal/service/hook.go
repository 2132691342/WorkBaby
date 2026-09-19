// Package service UserHookService：用户钩子 CRUD + 子进程执行器。
// 协议：一行 JSON 载荷进 stdin，JSON 决策出 stdout；退出码 0 = 解析 stdout、2 = 阻断、
// 其他 = 当前钩子可恢复失败。子进程失败 / 超时 / 非法输出一律放行（钩子故障不阻塞主流程）。
package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// hookTimeoutBounds 钩子超时上下限（毫秒）。
const (
	hookTimeoutDefault = 10000
	hookTimeoutMax     = 60000
	hookTimeoutMin     = 1000
	// hookStdoutMax 单次钩子标准输出读取上限（超出截断，防止子进程刷屏拖垮 run）。
	hookStdoutMax = 32 * 1024
	// stopHookMaxContinues Stop 钩子连续续跑上限：超过即放行收尾，防无限续跑。
	stopHookMaxContinues = 3
)

// UserHookService 钩子管理（设置页 CRUD + 运行时执行器一体）。
type UserHookService struct {
	repo *repo.UserHookRepo

	mu    sync.Mutex
	cache []domain.UserHookDO
	valid bool
}

// NewUserHookService 构造。
func NewUserHookService(repo *repo.UserHookRepo) *UserHookService {
	return &UserHookService{repo: repo}
}

// toHookRESP DO → RESP。
func toHookRESP(row *domain.UserHookDO) domain.UserHookRESP {
	return domain.UserHookRESP{
		ID: row.ID, Name: row.Name, Event: domain.NormalizeHookEvent(row.Event), Matcher: row.Matcher,
		Command: row.Command, TimeoutMs: row.TimeoutMs, Enabled: row.Enabled, Sort: row.Sort,
	}
}

// List 全量钩子。
func (s *UserHookService) List(ctx context.Context) ([]domain.UserHookRESP, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.UserHookRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toHookRESP(&rows[i]))
	}
	return out, nil
}

// Upsert 创建/更新（校验事件与命令；timeout 收敛到合法区间）。
func (s *UserHookService) Upsert(ctx context.Context, req *domain.UserHookREQ) (*domain.UserHookRESP, error) {
	name := strings.TrimSpace(req.Name)
	event := domain.NormalizeHookEvent(strings.TrimSpace(req.Event))
	command := strings.TrimSpace(req.Command)
	if name == "" {
		return nil, pkg.New(domain.ErrHookInvalid.Code, "钩子名称不能为空", "")
	}
	if !isHookEvent(event) {
		return nil, pkg.New(domain.ErrHookInvalid.Code, "非法的钩子事件", event)
	}
	if command == "" {
		return nil, pkg.New(domain.ErrHookInvalid.Code, "钩子命令不能为空", "")
	}
	matcher := strings.TrimSpace(req.Matcher)
	// 不参与 matcher 的事件不保留匹配式：留着会让人误以为生效
	if matcher != "" && !domain.HookEventUsesMatcher(event) {
		return nil, pkg.New(domain.ErrHookInvalid.Code, "该事件不支持 matcher", event)
	}
	if err := checkMatcher(matcher); err != nil {
		return nil, err
	}
	timeout := req.TimeoutMs
	if timeout == 0 {
		timeout = hookTimeoutDefault
	}
	if timeout < hookTimeoutMin {
		timeout = hookTimeoutMin
	}
	if timeout > hookTimeoutMax {
		timeout = hookTimeoutMax
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sort := 0
	if req.Sort != nil {
		sort = *req.Sort
	}

	row := &domain.UserHookDO{
		ID: req.ID, Name: name, Event: event, Matcher: matcher,
		Command: command, TimeoutMs: timeout, Enabled: enabled, Sort: sort,
	}
	if row.ID != "" {
		// 更新：目标必须存在，且保留创建时间
		existing, gerr := s.repo.GetByID(ctx, row.ID)
		if gerr != nil {
			return nil, gerr
		}
		row.CreatedAt = existing.CreatedAt
	} else {
		row.ID = pkg.NewID("HOOK")
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	s.invalidate()
	out := toHookRESP(row)
	return &out, nil
}

// Delete 删除钩子。
func (s *UserHookService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// Test 设置页试跑：以样例载荷执行一次，返回决策与耗时（不落库）。
func (s *UserHookService) Test(ctx context.Context, id string) (*domain.HookTestRESP, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !row.Enabled {
		return nil, pkg.New(domain.ErrHookTestFailed.Code, "钩子未启用", row.Name)
	}
	row.Event = domain.NormalizeHookEvent(row.Event)
	start := time.Now()
	decision, err := s.runOne(ctx, row, samplePayload(row.Event))
	perm, reason := decision.ToolPermission()
	return &domain.HookTestRESP{
		Decision:   firstNonEmpty(perm, decision.Decision, "allow"),
		Reason:     firstNonEmpty(reason, decision.Reason),
		Context:    decision.ContextText(),
		Err:        errString(err),
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// errString 错误文本；nil 返回空串（响应字段不用指针表达「无错误」）。
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// samplePayload 试跑载荷：按事件给出贴近真实的样例输入。
func samplePayload(event string) domain.HookPayload {
	p := domain.HookPayload{HookEventName: event, SessionID: "test", RunID: "test", Cwd: `C:\`}
	switch event {
	case domain.HookEventUserPromptSubmit:
		p.Prompt = "列出当前目录的文件"
	case domain.HookEventPreToolUse, domain.HookEventPermissionRequest, domain.HookEventPostToolUse:
		p.ToolName = "exec"
		p.ToolInput = json.RawMessage(`{"command":"echo hook-test"}`)
		p.ToolUseID = "tool-test"
		if event == domain.HookEventPostToolUse {
			p.ToolResponse = "hook-test\n"
		}
	case domain.HookEventPostToolUseFailure:
		p.ToolName = "exec"
		p.ToolInput = json.RawMessage(`{"command":"exit 1"}`)
		p.ToolUseID = "tool-test"
		p.Error = "exit status 1"
	case domain.HookEventStop:
		p.LastAssistantMessage = "已完成。"
	case domain.HookEventSessionStart:
		p.Source = "startup"
	}
	return p
}

// ---- 运行时执行器 ----

// enabledByEvent 启用且事件（已归一）匹配的钩子，按 sort 升序。
func (s *UserHookService) enabledByEvent(ctx context.Context, event string) []domain.UserHookDO {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.valid {
		rows, err := s.repo.List(ctx)
		if err != nil {
			pkg.L.Warn("hook cache load failed", "err", err.Error())
			return nil
		}
		s.cache = rows
		s.valid = true
	}
	out := make([]domain.UserHookDO, 0, 4)
	for _, h := range s.cache {
		h.Event = domain.NormalizeHookEvent(h.Event)
		if h.Enabled && h.Event == event {
			out = append(out, h)
		}
	}
	sortHooks(out)
	return out
}

// invalidate 配置变更后失效缓存（下次触发重载）。
func (s *UserHookService) invalidate() {
	s.mu.Lock()
	s.valid = false
	s.mu.Unlock()
}

// sortHooks 按 sort 升序稳定排序（sort 相同保持原顺序）。
func sortHooks(rows []domain.UserHookDO) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Sort < rows[j-1].Sort; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// matcherPlain 纯名称名单的合法字形：字母 / 数字 / 下划线 / 竖线。
var matcherPlain = regexp.MustCompile(`^[A-Za-z0-9_|]+$`)

// checkMatcher 校验 matcher 写法：空 / * / 名称名单 / 合法正则。
func checkMatcher(matcher string) error {
	if matcher == "" || matcher == "*" {
		return nil
	}
	if matcherPlain.MatchString(matcher) {
		return nil
	}
	if _, err := regexp.Compile(matcher); err != nil {
		return pkg.New(domain.ErrHookInvalid.Code, "非法的 matcher 正则", err.Error())
	}
	return nil
}

// hookMatches 工具名匹配：
// 空 / * = 全部；只含字母数字下划线与竖线 = 精确名称名单；其余按正则。
// 非法正则不执行该 matcher（fail-closed，与写入期校验一致）。
func hookMatches(matcher, toolName string) bool {
	m := strings.TrimSpace(matcher)
	switch {
	case m == "" || m == "*":
		return true
	case matcherPlain.MatchString(m):
		for _, name := range strings.Split(m, "|") {
			if strings.TrimSpace(name) == toolName {
				return true
			}
		}
		return false
	default:
		re, err := regexp.Compile(m)
		if err != nil {
			pkg.L.Warn("hook matcher regex invalid, skipped", "matcher", m, "err", err.Error())
			return false
		}
		return re.MatchString(toolName)
	}
}

// runOne 执行单个钩子子进程：stdin 一行 JSON 载荷 → stdout JSON 决策。
// 退出码 2 记入决策（阻断快捷方式）；其他非零视为可恢复失败，由调用方放行。
func (s *UserHookService) runOne(ctx context.Context, h *domain.UserHookDO, payload domain.HookPayload) (domain.HookDecision, error) {
	timeout := time.Duration(h.TimeoutMs) * time.Millisecond
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := newShellCmd(cctx, h.Command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return domain.HookDecision{}, err
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Start(); err != nil {
		return domain.HookDecision{}, err
	}
	bs, _ := json.Marshal(payload)
	_, _ = stdin.Write(append(bs, '\n'))
	_ = stdin.Close()

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	var errProc error
	select {
	case errProc = <-waitErr:
	case <-cctx.Done():
		errProc = cctx.Err()
	}

	decision := parseHookOutput(stdout.Bytes())
	decision.ExitCode = exitCodeOf(errProc)

	// 退出码语义：0 = 成功；2 = 阻断（已在 ToolPermission 中生效）；其他 = 可恢复失败
	if errProc != nil && decision.ExitCode != 2 {
		return decision, fmt.Errorf("%w: %s", errProc, clipRunes(lastLine(stdout.String()), 200))
	}
	if errors.Is(errProc, context.DeadlineExceeded) {
		return decision, errProc
	}
	return decision, nil
}

// parseHookOutput 解析子进程输出：取最后一行以 { 开头的合法 JSON。
// 非 JSON 输出只作诊断（不进入模型上下文），空输出表示「成功且无附加效果」。
// 退出码不在此处处理——由 runOne 统一赋给决策。
func parseHookOutput(raw []byte) domain.HookDecision {
	decision := domain.HookDecision{}
	if len(raw) > hookStdoutMax {
		raw = raw[:hookStdoutMax]
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), hookStdoutMax)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var d domain.HookDecision
		if err := json.Unmarshal([]byte(line), &d); err == nil {
			decision = d
		}
	}
	return decision
}

// exitCodeOf 提取进程退出码；非 ExitError（超时 / 未启动）返回 -1。
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// lastLine 取文本最后一行非空内容（诊断用）。
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if v := strings.TrimSpace(lines[i]); v != "" {
			return v
		}
	}
	return ""
}

// clipRunes 按字符截断（避免截断出半个中文字符）。
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// SessionStart 首轮模型请求前的钩子：返回要注入 system 的补充上下文（可空）。
func (s *UserHookService) SessionStart(ctx context.Context, sessionID, runID, source, cwd, permMode string) string {
	var parts []string
	for _, h := range s.enabledByEvent(ctx, domain.HookEventSessionStart) {
		hh := h
		out, err := s.runOne(ctx, &hh, domain.HookPayload{
			HookEventName: domain.HookEventSessionStart, SessionID: sessionID, RunID: runID,
			Source: source, Cwd: cwd, PermissionMode: permMode,
		})
		if err != nil {
			pkg.L.Warn("SessionStart hook failed (ignored)", "hook", hh.Name, "err", err.Error())
			continue
		}
		if txt := out.ContextText(); txt != "" {
			parts = append(parts, txt)
		}
	}
	return strings.Join(parts, "\n\n")
}

// UserPromptSubmit 用户提交后的钩子：block 阻断本次请求；context 注入本轮 system。
func (s *UserHookService) UserPromptSubmit(ctx context.Context, sessionID, runID, prompt, cwd, permMode string) (block bool, reason, context string) {
	var parts []string
	for _, h := range s.enabledByEvent(ctx, domain.HookEventUserPromptSubmit) {
		hh := h
		out, err := s.runOne(ctx, &hh, domain.HookPayload{
			HookEventName: domain.HookEventUserPromptSubmit, SessionID: sessionID, RunID: runID,
			Prompt: prompt, Cwd: cwd, PermissionMode: permMode,
		})
		if err != nil {
			pkg.L.Warn("UserPromptSubmit hook failed (ignored)", "hook", hh.Name, "err", err.Error())
			continue
		}
		if txt := out.ContextText(); txt != "" {
			parts = append(parts, txt)
		}
		// 退出码 2 或显式 continue=false 都阻断本次请求
		if out.ExitCode == 2 || (out.Continue != nil && !*out.Continue) {
			r := firstNonEmpty(out.Reason, "blocked by user hook: "+hh.Name)
			pkg.L.Info("UserPromptSubmit hook blocked request", "hook", hh.Name)
			return true, r, strings.Join(parts, "\n\n")
		}
	}
	return false, "", strings.Join(parts, "\n\n")
}

// PreToolUseResult PreToolUse 三态裁决与附加上下文。
type PreToolUseResult struct {
	Decision string // allow / ask / deny；空 = 无意见
	Reason   string
	Context  string // 放行时追加进模型可见的工具回执
}

// PreToolUse 工具执行前钩子：多钩子聚合 deny > ask > allow；附加上下文全部收集。
func (s *UserHookService) PreToolUse(ctx context.Context, sessionID, runID, toolName, toolCallID, cwd, permMode string, args json.RawMessage) PreToolUseResult {
	out := PreToolUseResult{}
	var contexts []string
	for _, h := range s.enabledByEvent(ctx, domain.HookEventPreToolUse) {
		if !hookMatches(h.Matcher, toolName) {
			continue
		}
		hh := h
		d, err := s.runOne(ctx, &hh, domain.HookPayload{
			HookEventName: domain.HookEventPreToolUse, SessionID: sessionID, RunID: runID,
			ToolName: toolName, ToolInput: args, ToolUseID: toolCallID,
			Cwd: cwd, PermissionMode: permMode,
		})
		if err != nil {
			pkg.L.Warn("PreToolUse hook failed (allowed)", "hook", hh.Name, "err", err.Error())
			continue
		}
		if txt := d.ContextText(); txt != "" {
			contexts = append(contexts, txt)
		}
		dec, reason := d.ToolPermission()
		switch dec {
		case "deny":
			// deny 立即短路：语义上不可能被后面的 allow 挽回
			pkg.L.Info("PreToolUse hook denied tool call", "hook", hh.Name, "tool", toolName)
			out.Decision, out.Reason = "deny", firstNonEmpty(reason, "denied by user hook: "+hh.Name)
			out.Context = strings.Join(contexts, "\n\n")
			return out
		case "ask":
			out.Decision, out.Reason = "ask", firstNonEmpty(reason, "requires confirmation by user hook: "+hh.Name)
		case "allow":
			if out.Decision == "" {
				out.Decision = "allow"
			}
		}
	}
	out.Context = strings.Join(contexts, "\n\n")
	return out
}

// PermissionRequest 权限询问前的钩子：返回 allow / deny 与理由；空 = 交回人工审批。
func (s *UserHookService) PermissionRequest(ctx context.Context, sessionID, runID, toolName, toolCallID, cwd, permMode string, args json.RawMessage) (decision, reason string) {
	for _, h := range s.enabledByEvent(ctx, domain.HookEventPermissionRequest) {
		if !hookMatches(h.Matcher, toolName) {
			continue
		}
		hh := h
		d, err := s.runOne(ctx, &hh, domain.HookPayload{
			HookEventName: domain.HookEventPermissionRequest, SessionID: sessionID, RunID: runID,
			ToolName: toolName, ToolInput: args, ToolUseID: toolCallID,
			Cwd: cwd, PermissionMode: permMode,
		})
		if err != nil {
			pkg.L.Warn("PermissionRequest hook failed (ignored)", "hook", hh.Name, "err", err.Error())
			continue
		}
		dec, r := d.ToolPermission()
		switch dec {
		case "deny":
			return "deny", firstNonEmpty(r, "denied by user hook: "+hh.Name)
		case "allow":
			return "allow", firstNonEmpty(r, "allowed by user hook: "+hh.Name)
		}
	}
	return "", ""
}

// PostToolUse 工具成功后钩子：返回追加进模型可见上下文的文本（可空）。
// 任一钩子出错只记日志，不影响工具结果本身。
func (s *UserHookService) PostToolUse(ctx context.Context, sessionID, runID, toolName, toolCallID, cwd, permMode string, args json.RawMessage, response string) string {
	return s.fireToolEvent(ctx, domain.HookEventPostToolUse, sessionID, runID, toolName, toolCallID, cwd, permMode, args, response, "")
}

// PostToolUseFailure 工具失败后钩子：返回恢复建议 / 诊断文本（可空）。
func (s *UserHookService) PostToolUseFailure(ctx context.Context, sessionID, runID, toolName, toolCallID, cwd, permMode string, args json.RawMessage, errText string) string {
	return s.fireToolEvent(ctx, domain.HookEventPostToolUseFailure, sessionID, runID, toolName, toolCallID, cwd, permMode, args, "", errText)
}

// fireToolEvent PostToolUse / PostToolUseFailure 共用的触发与上下文收集。
func (s *UserHookService) fireToolEvent(ctx context.Context, event, sessionID, runID, toolName, toolCallID, cwd, permMode string, args json.RawMessage, response, errText string) string {
	var parts []string
	for _, h := range s.enabledByEvent(ctx, event) {
		if !hookMatches(h.Matcher, toolName) {
			continue
		}
		hh := h
		d, err := s.runOne(ctx, &hh, domain.HookPayload{
			HookEventName: event, SessionID: sessionID, RunID: runID,
			ToolName: toolName, ToolInput: args, ToolUseID: toolCallID,
			ToolResponse: response, Error: errText,
			Cwd: cwd, PermissionMode: permMode,
		})
		if err != nil {
			pkg.L.Warn("tool hook failed (ignored)", "hook", hh.Name, "event", event, "err", err.Error())
			continue
		}
		if txt := d.ContextText(); txt != "" {
			parts = append(parts, txt)
		}
	}
	return strings.Join(parts, "\n\n")
}

// StopResult Stop 钩子决策：Block 为真时主循环带着 Reason 再跑一轮。
type StopResult struct {
	Block  bool
	Reason string
}

// stopFeedback Stop 决策是否要求续跑：返回反馈文本与是否续跑。
// 续跑必须带反馈文本——没有反馈的 block 会让主循环原地空转一轮。
func stopFeedback(d domain.HookDecision) (string, bool) {
	feedback := firstNonEmpty(d.Reason, d.ContextText())
	if feedback == "" {
		return "", false
	}
	if d.Decision == "block" || d.ExitCode == 2 || (d.Continue != nil && *d.Continue) {
		return feedback, true
	}
	return "", false
}

// Stop 模型准备结束时钩子：返回 block 让主循环续跑一轮（对齐协议：需带 reason / 上下文）。
// stopActive 表示本次 Stop 由上一次续跑触发，用于避免无限续跑。
func (s *UserHookService) Stop(ctx context.Context, sessionID, runID, lastAssistant string, stopActive bool) StopResult {
	for _, h := range s.enabledByEvent(ctx, domain.HookEventStop) {
		hh := h
		d, err := s.runOne(ctx, &hh, domain.HookPayload{
			HookEventName: domain.HookEventStop, SessionID: sessionID, RunID: runID,
			LastAssistantMessage: clipRunes(lastAssistant, 4000), StopHookActive: stopActive,
		})
		if err != nil {
			pkg.L.Warn("Stop hook failed (ignored)", "hook", hh.Name, "err", err.Error())
			continue
		}
		if feedback, block := stopFeedback(d); block {
			pkg.L.Info("Stop hook requested another turn", "hook", hh.Name)
			return StopResult{Block: true, Reason: feedback}
		}
	}
	return StopResult{}
}

// 工具类钩子的消费点在 service.hookGuard（core 中间件链），本服务只提供执行原语。

// isHookEvent 事件是否在合法集合内。
func isHookEvent(event string) bool {
	for _, e := range domain.HookEvents {
		if e == event {
			return true
		}
	}
	return false
}
