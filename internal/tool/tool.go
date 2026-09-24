// Package tool 是 WorkBaby 工具系统：统一 Tool 抽象、注册中心、参数校验与执行策略。
//
// 边界：tool/ 不依赖 harness / agent / api / service / wails；
// 通过构造注入依赖 rag.Retriever / memory.Service 等；
// 工具自身不落库（消息落库由 harness/service 负责）。
package tool

import (
	"context"
	"encoding/json"
	"strings"
)

// RiskLevel 工具风险分级（用于审批与前端展示）。
type RiskLevel string

const (
	RiskReadOnly    RiskLevel = "readonly"    // 读文件 / 检索
	RiskWriteLocal  RiskLevel = "write_local" // 写本地文件
	RiskExec        RiskLevel = "exec"        // 执行命令
	RiskNetwork     RiskLevel = "network"     // 访问网络
	RiskDestructive RiskLevel = "destructive" // 删文件 / 删数据
)

// ToolSchema 暴露给 LLM 的工具描述。
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ToolResult 工具执行结果；Content 回填给 LLM，Data 供前端结构化展示。
type ToolResult struct {
	Content string            // 文本结果
	Data    map[string]any    // 结构化数据（可选）
	Err     error             // 执行失败（非 nil 时 Content 仍可携带错误说明）
	Meta    map[string]string // 耗时 / exitCode 等元数据
	Refused bool              // 审批拒绝（Refused 语义：非故障，不推进失败熔断）
}

// MetaTerminate 工具结果元信息键：置 "1"（或 "true"）表示「本结果已是终局」——跳过下一轮
// 模型调用直接 end_turn 收尾。同一轮**全部**结果都置位才生效：混合批次里只要有一个结果
// 需要模型继续处理，就照常进入下一轮。
const MetaTerminate = "terminate"

// IsTerminal 工具结果是否自述已是终局。
func IsTerminal(r ToolResult) bool {
	v := strings.TrimSpace(r.Meta[MetaTerminate])
	return v == "1" || strings.EqualFold(v, "true")
}

// Tool 统一工具接口。
type Tool interface {
	Name() string
	Description() string
	Schema() ToolSchema
	RiskLevel() RiskLevel
	Execute(ctx context.Context, args json.RawMessage) ToolResult
}

// ToolMeta 声明式工具元信息。
// 权限裁决、只读并发、前端呈现统一由元数据驱动，新增工具零硬编码分支。
// RiskLevel 仍是审批的权威口径；Meta 提供更细的执行与呈现信号。
type ToolMeta struct {
	ReadOnly       bool     // 纯读，无副作用（可参与只读并行执行）
	Destructive    bool     // 不可逆操作（删除 / 覆盖），前端红色标记
	PathParams     []string // 指向文件系统路径的参数名（沙箱越界校验 + 前端路径展示）
	TimeoutSec     int      // 建议单次执行超时；0 = 用 harness 默认
	MaxResultChars int      // 结果回填 LLM 前的建议截断长度；0 = 用 harness 默认
	UIHint         string   // 前端呈现提示（如 "editor" / "browser" / "diff"）；空 = 默认时间线样式
	Group          string   // 展示分组：file / exec / doc / agent / media；空 = 由名称与风险推导
	// Category 动作类别：info / edit / exec / network / irreversible。
	// 前端据此选图标与措辞，不必再按工具名逐条硬编码（新增工具零改动）。
	Category string
	// ActivityDesc 静态动作短语（"正在读取文件"）；配套 ActivityProvider 可给出带参数的描述。
	ActivityDesc string
}

// 动作类别。
const (
	CategoryInfo         = "info"
	CategoryEdit         = "edit"
	CategoryExec         = "exec"
	CategoryNetwork      = "network"
	CategoryIrreversible = "irreversible"
)

// ActivityProvider 可选接口：按本次参数给出面向用户的一行动作描述
// （"正在编辑 src/app.ts"）——比前端维护「工具名 → 措辞」映射表准确得多。
type ActivityProvider interface {
	ActivityDescription(args json.RawMessage) string
}

// ActivityOf 取本次调用的动作描述：工具自述 > 元数据静态短语 > 通用兜底。
func ActivityOf(t Tool, args json.RawMessage) string {
	if t == nil {
		return ""
	}
	if ap, ok := t.(ActivityProvider); ok {
		if s := ap.ActivityDescription(args); s != "" {
			return s
		}
	}
	if m := MetaOf(t); m.ActivityDesc != "" {
		return m.ActivityDesc
	}
	return "调用 " + t.Name()
}

// CategoryOf 动作类别（MetaOf 已保证非空）。
func CategoryOf(t Tool) string { return MetaOf(t).Category }

// 展示分组：与前端工具页分组顺序一致。
const (
	GroupFile      = "file"
	GroupExec      = "exec"
	GroupDoc       = "doc"
	GroupAgent     = "agent"
	GroupMedia     = "media"
	GroupFunctools = "functools" // 纯函数工具（默认禁用，避免 Agent 工具集过载）
)

// GroupOf 按工具名前缀推导展示分组，未命中再按风险等级兜底。
// 新增工具无需声明：前缀规则覆盖不到时落到 media（文本·数据·多媒体）。
func GroupOf(name string, risk RiskLevel) string {
	switch {
	case hasPrefix(name, "file_", "edit_", "patch_"):
		return GroupFile
	case hasPrefix(name, "exec", "skill_run", "http_", "web_"):
		return GroupExec
	case hasPrefix(name, "doc_", "pdf_", "archive_", "knowledge_", "rag_"):
		return GroupDoc
	case hasPrefix(name, "delegate", "todo_", "request_input", "memory_", "plan_"):
		return GroupAgent
	}
	switch risk {
	case RiskExec, RiskNetwork:
		return GroupExec
	case RiskWriteLocal, RiskDestructive:
		return GroupFile
	case RiskReadOnly:
		return GroupDoc
	}
	return GroupMedia
}

func hasPrefix(name string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// MetaProvider 可选接口：实现它的工具可提供声明式元信息（未实现走零值兜底）。
// 以可选接口而非直接扩 Tool，避免破坏既有 13 个工具包。
type MetaProvider interface {
	Meta() ToolMeta
}

// MetaOf 读取工具元信息：实现了 MetaProvider 用声明值，否则按 RiskLevel 兜底推导。
func MetaOf(t Tool) ToolMeta {
	m := ToolMeta{}
	if mp, ok := t.(MetaProvider); ok {
		m = mp.Meta()
	} else {
		switch t.RiskLevel() {
		case RiskReadOnly, RiskNetwork:
			m.ReadOnly = true
		case RiskDestructive:
			m.Destructive = true
		}
	}
	if m.Group == "" {
		m.Group = GroupOf(t.Name(), t.RiskLevel())
	}
	if m.Category == "" {
		m.Category = deriveCategory(m, t.RiskLevel())
	}
	return m
}

// deriveCategory 未声明类别时按只读/破坏性/风险推导。
func deriveCategory(m ToolMeta, risk RiskLevel) string {
	switch {
	case m.Destructive || risk == RiskDestructive:
		return CategoryIrreversible
	case risk == RiskExec:
		return CategoryExec
	case risk == RiskNetwork:
		return CategoryNetwork
	case m.ReadOnly || risk == RiskReadOnly:
		return CategoryInfo
	default:
		return CategoryEdit
	}
}
