package tool

import "WorkBaby/internal/pkg"

// 错误变量。
var (
	ErrToolNotFound   = pkg.New(4005, "tool not found", "")
	ErrToolDisabled   = pkg.New(4005, "tool disabled", "")
	ErrArgsInvalid    = pkg.New(4004, "args do not match tool schema", "")
	ErrBinaryDenied   = pkg.New(4001, "binary not allowed by policy", "")
	ErrPatternDenied  = pkg.New(4002, "command matches denied pattern", "")
	ErrApprovalNeeded = pkg.New(4003, "approval required", "")
	ErrToolTimeout    = pkg.New(4006, "tool execution timeout", "")
	ErrPathEscape     = pkg.New(4007, "path escapes workspace root", "")
)
