// Package doc 提供文档解析工具（pdf_reader / word_reader / excel_reader）。
//
// 复用 rag.Loader 抽取文本（ledongthuc/pdf 已引入）；路径强制限定在 workspace 根内
// 。
package doc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/rag"
	"WorkBaby/internal/tool"
)

// Reader 文档读取工具：按扩展名抽取文本。
type Reader struct {
	resolve func(context.Context) string
}

// New 构造文档读取工具；resolver 按会话解析 workspace 根（路径白名单），defRoot 为回落根。
func New(resolver tool.RootResolver, defRoot string) *Reader {
	return &Reader{resolve: tool.ResolveRoot(resolver, defRoot)}
}

func (t *Reader) Name() string              { return "doc_reader" }
func (t *Reader) RiskLevel() tool.RiskLevel { return tool.RiskReadOnly }
func (t *Reader) Description() string {
	return "抽取文档（pdf/docx/txt/md/html/csv/json）的纯文本。路径必须是工作区内文件。"
}

func (t *Reader) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["path"],
			"properties": {
				"path": {"type": "string", "description": "工作区内文档文件路径（绝对或相对）"}
			}
		}`),
	}
}

type docReq struct {
	Path string `json:"path"`
}

// Execute 解析文档并返回纯文本。
func (t *Reader) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req docReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "doc_reader args parse failed", err)}
	}
	abs, err := t.safePath(t.resolve(ctx), req.Path)
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	if _, err := os.Stat(abs); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4007, "doc_reader file not found", err)}
	}
	loader := rag.PickLoader(rag.MIMEFromPath(abs))
	if loader == nil {
		return tool.ToolResult{Err: pkg.New(4004, "doc_reader unsupported file type", req.Path)}
	}
	text, err := loader.Load(context.Background(), abs)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "doc_reader extract failed", err)}
	}
	return tool.ToolResult{Content: text}
}

// safePath 校验路径在 workspace 根内并绝对化。
func (t *Reader) safePath(workspaceRoot, path string) (string, error) {
	if path == "" {
		return "", tool.ErrPathEscape
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", pkg.Wrap(4007, "abs root failed", err)
	}
	candidate := path
	if !filepath.IsAbs(path) {
		candidate = filepath.Join(root, path)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", pkg.Wrap(4007, "abs path failed", err)
	}
	if err := pkg.Within(root, abs); err != nil {
		return "", tool.ErrPathEscape
	}
	return abs, nil
}
