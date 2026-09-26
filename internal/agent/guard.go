package agent

import (
	"context"
	"encoding/json"
	"regexp"
	"sync"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// 拒绝原因码：护栏中间件统一用它填 ToolResultPayload.RefusedReason，前端与上层可编程式反应。
const (
	RefuseNotExposed = "not_exposed"
	RefuseInjection  = "prompt_injection"
	RefusePathTrust  = "path_trust"
	RefusePolicy     = "policy"
	RefuseApproval   = "approval"
	RefuseLoopGuard  = "loop_guard"
)

// injectionPattern 伪工具调用标记：模型在参数里塞入伪造的 tool_call / tool_result 块试图污染回执。
var injectionPattern = regexp.MustCompile(`(?i)<\|?(tool_call|tool_result|function_call|im_start|im_end)\s*/?\s*\|?>`)

// refuse 构造拒绝回执。拒绝不是故障：不带 Err，模型可据此改道续跑。
func refuse(reason, hint string) tool.ToolResult {
	return tool.ToolResult{Content: hint, Refused: true, Meta: map[string]string{"refused_reason": reason}}
}

// ExposeGuard 未在本次暴露清单中的工具直接拒绝。
// allowed 为空表示不设白名单（全部已注册工具可用）。
func ExposeGuard(allowed map[string]bool) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, call Call) tool.ToolResult {
			if allowed != nil && !allowed[call.Name] {
				return refuse(RefuseNotExposed, "工具未在本轮暴露清单中: "+call.Name)
			}
			if call.Tool == nil {
				return refuse(RefuseNotExposed, "工具未注册: "+call.Name)
			}
			return next(ctx, call)
		}
	}
}

// SchemaGuard 参数 JSON Schema 校验；不合规按工具错误回填（模型可修正后重试）。
func SchemaGuard() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, call Call) tool.ToolResult {
			if call.Tool == nil {
				return next(ctx, call)
			}
			if err := tool.ValidateArgs(call.Tool.Schema().Parameters, call.Args); err != nil {
				return tool.ToolResult{
					Content: "参数不合规: " + err.Error(),
					Err:     pkg.Wrap(4004, "工具参数校验失败", err),
				}
			}
			return next(ctx, call)
		}
	}
}

// Mode 会话权限模式；决定各风险档的默认裁决。
type Mode string

const (
	ModeDefault  Mode = "default"   // 只读直行，其余询问
	ModeAutoEdit Mode = "auto_edit" // 只读与本地写直行，执行/网络/删除询问
	ModeYolo     Mode = "yolo"      // 全部放行；destructive 仍询问（不可逆永不免审）
)

// Decision 策略裁决结果。
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

// Decide 按风险档给出默认裁决；destructive 恒为 ask（不可逆，永不免审）。
func (m Mode) Decide(risk tool.RiskLevel) Decision {
	switch m {
	case ModeYolo:
		if risk == tool.RiskDestructive {
			return DecisionAsk
		}
		return DecisionAllow
	case ModeAutoEdit:
		switch risk {
		case tool.RiskReadOnly, tool.RiskWriteLocal:
			return DecisionAllow
		default:
			return DecisionAsk
		}
	default: // ModeDefault
		if risk == tool.RiskReadOnly {
			return DecisionAllow
		}
		return DecisionAsk
	}
}

// Rules 显式策略规则（设置页配置）；优先级高于模式矩阵。命中返回 ok=true。
type Rules interface {
	Match(name string) (Decision, bool)
}

// Approver 人工审批门：阻塞等待决策，返回 true 放行。
type Approver interface {
	Approve(ctx context.Context, call Call, risk string) bool
}

// ApprovalReasoner 可选接口：审批门自带拒绝原因，拒绝回执据此可解释
// （默认回执「用户拒绝执行」对后台任务这类无人工介入的门是误导）。
type ApprovalReasoner interface {
	DenyReason() string
}

// PathPolicy 路径/计划模式预检：返回 ok=true 放行；false 则 reason 写入拒绝回执。
// 调用方（service 层）用其封装工作区信任 + 计划模式只读检查。
type PathPolicy func(ctx context.Context, call Call) (ok bool, reason string)

// PolicyGuard 一站式安全门：注入检测 → 路径/计划预检 → 风险×模式×规则 → 人工审批。
// 已标记 Trusted 的调用直接放行（消除双层重复询问）。
func PolicyGuard(mode Mode, rules Rules, approver Approver, pathPolicy PathPolicy) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, call Call) tool.ToolResult {
			if call.Trusted || call.Tool == nil {
				return next(ctx, call)
			}
			// 1) 参数内容安全（注入防护）
			if injectionPattern.Match(call.Args) {
				return refuse(RefuseInjection, "参数含伪工具调用标记，已拒绝执行")
			}
			// 2) 路径/计划模式预检
			if pathPolicy != nil {
				if ok, reason := pathPolicy(ctx, call); !ok {
					return refuse(RefusePathTrust, reason)
				}
			}
			// 3) 风险×模式×规则×审批
			risk := call.Tool.RiskLevel()
			decision := mode.Decide(risk)
			if rules != nil {
				if d, ok := rules.Match(call.Name); ok {
					decision = d
				}
			}
			switch decision {
			case DecisionAllow:
				return next(ctx, call)
			case DecisionDeny:
				return refuse(RefusePolicy, "策略拒绝执行: "+call.Name)
			default:
				if approver == nil {
					return refuse(RefuseApproval, "需要审批但未接入审批门: "+call.Name)
				}
				if !approver.Approve(ctx, call, riskOf(risk)) {
					hint := "用户拒绝执行: " + call.Name
					if r, ok := approver.(ApprovalReasoner); ok && r.DenyReason() != "" {
						hint = r.DenyReason() + ": " + call.Name
					}
					return refuse(RefuseApproval, hint)
				}
				return next(ctx, call)
			}
		}
	}
}

// riskOf 把工具风险档映射为审批语义（与前端 ApprovalRequest.risk 对齐）。
func riskOf(risk tool.RiskLevel) string {
	if risk == tool.RiskDestructive {
		return tool.RiskApprovalIrrev
	}
	return tool.RiskApprovalNeeds
}

// StepStore 已完成工具调用的结果记忆；用于续跑时复用，不重放副作用。
type StepStore interface {
	Load(key string) (string, bool)
	Store(key, content string)
}

// counter 一次 run 内同名同参的调用计数；streak 为连续相同 key 的次数，total 为累计次数。
// 同参异序经 canonicalArgs 规范化后仍判为同一 key。
type counter struct {
	mu        sync.Mutex
	streak    map[string]int
	total     map[string]int
	streakKey string
	limit     int
}

func newCounter(limit int) *counter {
	if limit <= 0 {
		limit = 3
	}
	return &counter{streak: map[string]int{}, total: map[string]int{}, limit: limit}
}

func (c *counter) tick(key string) (streak, total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if key == c.streakKey {
		c.streak[key]++
	} else {
		c.streak = map[string]int{key: 1}
		c.streakKey = key
	}
	c.total[key]++
	return c.streak[key], c.total[key]
}

// RepeatGuard 重复调用熔断 + 已完成步骤复用：把原 LoopGuard 与 IdempotentGuard 合并为一个中间件。
//
// 顺序：复用 → 计数 → 检查 → 执行 → 记录。重用命中时直接返回缓存，不增计数也不消耗审批配额。
func RepeatGuard(limit int, store StepStore) Middleware {
	c := newCounter(limit)
	return func(next Handler) Handler {
		return func(ctx context.Context, call Call) tool.ToolResult {
			key := call.Name + "|" + canonicalArgs(call.Args)
			// 1) 重用：续跑/重发场景不重放副作用。
			if store != nil {
				if content, ok := store.Load(key); ok {
					return tool.ToolResult{Content: content, Meta: map[string]string{"idempotent": "reused"}}
				}
			}
			// 2) 计数：超过熔断阈值直接拒绝（前序拒绝不计数，因为计数发生在计数前）。
			streak, total := c.tick(key)
			if streak >= c.limit || total > c.limit {
				return refuse(RefuseLoopGuard, "检测到重复调用（连续 "+itoa(streak)+" 次 / 累计 "+itoa(total)+" 次），请换一种方式")
			}
			// 3) 执行。
			res := next(ctx, call)
			// 4) 成功后入库：失败或拒绝的副作用不可重用。
			if store != nil && res.Err == nil && !res.Refused {
				store.Store(key, res.Content)
			}
			return res
		}
	}
}

// canonicalArgs 参数规范化：同参异序仍判为同一次调用。
func canonicalArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return "{}"
	}
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return string(args)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(args)
	}
	return string(b)
}

// itoa 小整数转字符串，避免为计数引入 strconv 噪音。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// failureTracker 按工具名跟踪 run 内的连续失败计数；任意一次成功即清零。
// 仅记"失败"，不记"拒绝"——拒绝是策略回执不是故障，模型必须按拒绝原因改道。
type failureTracker struct {
	mu     sync.Mutex
	streak map[string]int
}

func newFailureTracker() *failureTracker {
	return &failureTracker{streak: map[string]int{}}
}

func (f *failureTracker) record(name string, failed bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if failed {
		f.streak[name]++
		return f.streak[name]
	}
	delete(f.streak, name)
	return 0
}

// AdaptiveLoopGuard 失败改道闸门：同一工具连续失败达阈值时，在 Content 末尾追加改道提示
// （换参数 / 换工具 / 向用户咨询）。未达阈值只把次数写进 Meta["same_failure_count"]；
// 任意一次成功即清零。拒绝不算失败——它是策略回执，不是故障。
func AdaptiveLoopGuard(threshold int) Middleware {
	if threshold <= 0 {
		threshold = 3
	}
	tracker := newFailureTracker()
	return func(next Handler) Handler {
		return func(ctx context.Context, call Call) tool.ToolResult {
			res := next(ctx, call)
			failed := res.Err != nil
			count := tracker.record(call.Name, failed)
			if !failed || count < threshold {
				if res.Meta == nil {
					res.Meta = map[string]string{}
				}
				res.Meta["same_failure_count"] = itoa(count)
				return res
			}
			// 同工具连续失败达到阈值：摘要 + 改道提示并入回执。
			hint := adaptiveHint(call, res, count)
			if hint != "" {
				res.Content = res.Content + "\n\n[guard] " + hint
			}
			if res.Meta == nil {
				res.Meta = map[string]string{}
			}
			res.Meta["same_failure_count"] = itoa(count)
			res.Meta["adaptive_hint"] = "1"
			return res
		}
	}
}

// adaptiveHint 生成改道提示：区分只读 vs 写/执行，提示后续可走的路径。
// 不强行换工具——把候选路径列给模型，让它结合上下文判断。
func adaptiveHint(call Call, res tool.ToolResult, count int) string {
	var risk tool.RiskLevel
	if call.Tool != nil {
		risk = call.Tool.RiskLevel()
	}
	base := "工具 " + call.Name + " 已连续失败 " + itoa(count) + " 次。请换一种方式："
	switch risk {
	case tool.RiskReadOnly, "":
		return base + "\n- 改用 file_read / file_grep / file_glob 直接探查\n- 改用 doc_reader 抽取文件正文\n- 仍需外部数据时调 delegate_task 派 explore 子 Agent\n- 资料确实缺失时向用户说明，不要死磕"
	case tool.RiskExec:
		return base + "\n- 检查命令参数（路径 / 引号 / 工作目录）\n- 用 file_list 先确认目标存在\n- 仍失败时向用户报告当前可见的症状（错误码 + 关键 stderr 一行）"
	case tool.RiskNetwork:
		return base + "\n- 检查 URL / 参数格式\n- 改用 webfetch 直拉正文\n- 多次失败后向用户报告"
	default:
		return base + "\n- 仔细核对入参\n- 改用相关只读工具先验证前置条件\n- 多次失败时向用户报告"
	}
}