package service

import (
	"html"
	"strings"
	"time"

	"WorkBaby/backend/domain"
)

// nowISO 产出引擎侧认的时间串（毫秒精度，UTC）。
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// renderSessionHTML 把历史轮渲染成单文件 HTML：样式内联，双击就能看。
func renderSessionHTML(title string, history []domain.HistoryTurnVO) string {
	var body strings.Builder
	for _, turn := range history {
		label := "系统"
		switch turn.TurnRole {
		case "user":
			label = "用户"
		case "assistant":
			label = "助手"
		case "tool":
			label = "工具"
		}
		body.WriteString(`<section class="` + turn.TurnRole + `"><h3>` + html.EscapeString(label))
		if turn.TurnModel != "" {
			body.WriteString(` <small>` + html.EscapeString(turn.TurnModel) + `</small>`)
		}
		body.WriteString(`</h3>`)
		for _, block := range turn.TurnBlocks {
			switch block.BlockKind {
			case domain.BlockTool:
				body.WriteString(`<div class="tool"><b>` + html.EscapeString(block.ToolName) + `</b><pre>` +
					html.EscapeString(block.ToolOutput) + `</pre></div>`)
			case domain.BlockThinking:
				body.WriteString(`<div class="think">` + html.EscapeString(block.BlockText) + `</div>`)
			default:
				body.WriteString(`<div class="text">` + html.EscapeString(block.BlockText) + `</div>`)
			}
		}
		body.WriteString(`</section>`)
	}
	return `<!doctype html><html lang="zh"><head><meta charset="utf-8"><title>` + html.EscapeString(title) +
		`</title><style>` + sessionHTMLStyle + `</style></head><body><h1>` + html.EscapeString(title) +
		`</h1>` + body.String() + `</body></html>`
}

// sessionHTMLStyle 是导出页的内联样式：导出文件要能离线单独打开。
const sessionHTMLStyle = `body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;max-width:880px;` +
	`margin:24px auto;padding:0 16px;color:#1e293b;line-height:1.6}` +
	`section{margin:18px 0;padding:12px 16px;border-radius:10px;background:#f8fafc}` +
	`section.user{background:#e0e7ff}` +
	`h3{margin:0 0 8px;font-size:13px;color:#6366f1;letter-spacing:.08em;text-transform:uppercase}` +
	`pre{white-space:pre-wrap;word-break:break-word;background:#fff;padding:8px;border-radius:6px;margin:6px 0}` +
	`.think{color:#64748b;font-style:italic}` +
	`small{color:#64748b;font-weight:400;letter-spacing:0;text-transform:none}`
