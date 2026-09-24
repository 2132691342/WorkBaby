package tool

import (
	"encoding/json"

	"WorkBaby/internal/pkg"
)

// DecodeArgs 解析工具入参到 dst；失败返回可直接返回给调用方的 4004 结果。
// 参数形状是模型与工具之间的协议，错在这里必须让模型看到并自行修正。
func DecodeArgs(args json.RawMessage, dst any) ToolResult {
	if err := json.Unmarshal(args, dst); err != nil {
		return ToolResult{Err: pkg.Wrap(4004, "工具参数解析失败", err)}
	}
	return ToolResult{}
}
