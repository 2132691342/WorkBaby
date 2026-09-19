package tool

import (
	"path"
	"strings"
)

// Decision 工具调用决策。
type Decision uint8

const (
	DecisionDeny  Decision = iota // 拒绝：不执行，模型收到可见拒绝原因
	DecisionAsk                   // 询问：人工审批后放行
	DecisionAllow                 // 放行
)

// SessionMode 会话权限模式（前端顶栏可切换；默认 default）。
type SessionMode string

const (
	// SessionModeRestricted 受限：只读直行，写/执行/网络/删除一律拒绝（不询问，模型可见拒绝原因自行改道）。
	SessionModeRestricted SessionMode = "restricted"
	// SessionModeDefault 需确认：只读直行，写/执行/网络/删除询问。
	SessionModeDefault SessionMode = "default"
	// SessionModeAutoEdit 自动：只读与本地写直行，执行/删除/网络询问。
	SessionModeAutoEdit SessionMode = "auto_edit"
	// SessionModeYolo 完全访问：全部放行（含命令执行与删除），适用于完全可信的批量任务。
	SessionModeYolo SessionMode = "yolo"
)

// ParseSessionMode 归一化权限模式字符串；未知值回落 default（不因脏数据放行）。
func ParseSessionMode(v string) SessionMode {
	switch SessionMode(strings.TrimSpace(v)) {
	case SessionModeRestricted:
		return SessionModeRestricted
	case SessionModeAutoEdit:
		return SessionModeAutoEdit
	case SessionModeYolo:
		return SessionModeYolo
	default:
		return SessionModeDefault
	}
}

// DefaultDecision 按 SessionMode × 工具风险给默认决策（Gate 未显式配置工具时用）。
func DefaultDecision(mode SessionMode, risk RiskLevel) Decision {
	switch mode {
	case SessionModeYolo:
		return DecisionAllow
	case SessionModeRestricted:
		if risk == RiskReadOnly {
			return DecisionAllow
		}
		return DecisionDeny
	case SessionModeAutoEdit:
		if risk == RiskReadOnly || risk == RiskWriteLocal {
			return DecisionAllow
		}
		return DecisionAsk
	default:
		if risk == RiskReadOnly {
			return DecisionAllow
		}
		return DecisionAsk
	}
}

// Gate 工具策略门：显式规则（按工具名，glob）优先，无规则回退 SessionMode × 风险默认。
//
// 分工（CLAUDE.md §6）：
//   - Gate 是 harness 执行层的「工具级」裁决（在工具调用前）；
//   - exec 工具内部仍是「命令级」白名单/危险正则（ExecPolicy），两者互不替代。
type Gate struct {
	mode  SessionMode
	rules map[string]Decision
}

// NewGate 构造工具策略门。
func NewGate(mode SessionMode) *Gate {
	return &Gate{mode: mode, rules: map[string]Decision{}}
}

// Allow 显式放行某工具（支持 glob）。
func (g *Gate) Allow(name string) *Gate { return g.Set(name, DecisionAllow) }

// Ask 显式要求某工具审批。
func (g *Gate) Ask(name string) *Gate { return g.Set(name, DecisionAsk) }

// Deny 显式禁止某工具。
func (g *Gate) Deny(name string) *Gate { return g.Set(name, DecisionDeny) }

// Set 设置规则；空 name 或非法模式忽略。
func (g *Gate) Set(name string, d Decision) *Gate {
	name = strings.TrimSpace(name)
	if name != "" {
		g.rules[name] = d
	}
	return g
}

// Mode 返回策略门的会话权限模式；nil 门按 default 处理。
//
// YOLO（完全访问）下用户已授权全部动作，调用方据此连命令级裁决一起跳过——
// 否则 Allow 与 YOLO 无法区分（两者 Decide 都返回 DecisionAllow）。
func (g *Gate) Mode() SessionMode {
	if g == nil {
		return SessionModeDefault
	}
	return g.mode
}

// Decide 返回该工具本轮决策：先精确/glob 显式规则，无规则走模式默认。
func (g *Gate) Decide(name string, risk RiskLevel) Decision {
	if g == nil {
		return DecisionAllow
	}
	if d, ok := g.lookup(name); ok {
		return d
	}
	return DefaultDecision(g.mode, risk)
}

// lookup 精确名优先，其次最长的 glob 规则。
func (g *Gate) lookup(name string) (Decision, bool) {
	if d, ok := g.rules[name]; ok {
		return d, true
	}
	best := ""
	for pat := range g.rules {
		if !strings.ContainsAny(pat, "*?[") {
			continue
		}
		if ok, _ := path.Match(pat, name); ok && len(pat) > len(best) {
			best = pat
		}
	}
	if best != "" {
		return g.rules[best], true
	}
	return DecisionDeny, false
}
