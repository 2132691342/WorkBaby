package service

import (
	"encoding/json"
)

// 本文件：工具策略门与 run 事件发射（emit 是 service → SSE 的唯一出口）。

// gateInternalAllowTools 工具策略门显式放行清单（default 模式下本会走 ask 的那些）。
//
// 放行不等于无护栏：实现了 tool.RiskClassifier 的工具（exec / run_skill_script）仍由
// runner 拿 ClassifyArgs 的 per-call 风险做命令级裁决——白名单外与危险正则命中照样弹审批。
var gateInternalAllowTools = []string{
	"exec", "run_skill_script", "delegate_task", // 命令级裁决走 RiskClassifier
	"websearch", "webfetch", "http", // 信息型网络读取
	"knowledge_search", "doc_reader", "todo", // 只读 / 会话内计划
	"file_write", "archive_manager", "memory_write", // 工作区沙箱内的本地写
}

// extractTargetDir 从工具入参抽取目标目录。
//
// 返回 (dir, ok)；ok=false 表示该工具与目录无关，调用方应跳过信任检查。
func extractTargetDir(toolName string, args json.RawMessage) (string, bool) {
	switch toolName {
	case "exec":
		// cwd 优先；为空 → 走进程默认目录（app home），跳过信任检查
		var v struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(args, &v); err != nil {
			return "", true
		}
		return v.Cwd, true
	case "file_read", "file_write", "file_list", "doc_reader":
		// 这些工具已被工作区根沙箱约束，目录信任在工具内部处理
		return "", false
	case "archive_manager":
		// archive_manager 的 source/target 是工作区相对路径，不涉及外部目录
		return "", false
	default:
		return "", false
	}
}

// todoToolName 计划工具名；其结构化产出单独发 chat:todo 事件（前端进度卡）。
const todoToolName = "todo"

// emit 发布 run 事件：统一走 Emitter（注入归属、分配 seq、广播）。
// 载荷支持 map[string]any（高频增量）或领域事件结构体（如 domain.ChatDoneEvent）；
// 字段一律 snake_case（CLAUDE.md §2.5）。
func (s *ChatService) emit(runID, sessionID, name string, payload any) {
	s.emitter.Emit(runID, sessionID, name, payload)
}
