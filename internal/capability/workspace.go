package capability

import (
	"context"
	"strings"

	"WorkBaby/internal/core"
	"WorkBaby/internal/tool"
)

// workspaceCap 注入会话绑定的工作目录，让「当前项目 / 这个目录」类指令有明确落点。
type workspaceCap struct{}

// NewWorkspace 构造工作区能力。
func NewWorkspace() Capability { return &workspaceCap{} }

func (c *workspaceCap) ID() string { return "workspace" }

func (c *workspaceCap) Preload(_ context.Context, p *PreloadCtx) ([]core.Section, error) {
	if p.Session == nil {
		return nil, nil
	}
	wp := strings.TrimSpace(p.Session.WorkspacePath)
	if wp == "" {
		return nil, nil
	}
	return []core.Section{{
		Key:   "workspace",
		Title: "当前工作目录",
		Body: "本会话绑定的本地工作目录：" + wp + "\n" +
			"file_read / file_write / file_list / doc_reader / archive 等文件工具与 exec 命令默认在此目录下工作，相对路径均基于此目录解析。" +
			"用户提到「当前项目 / 当前工作区 / 这个目录」时即指此处。\n" +
			"落点纪律：" + wp + " 是用户的目录，只写用户要的产物。中间脚本、临时文件、分析报告、导出结果一律放进 " +
			wp + "/.workbaby/ 下（scripts/ 过程脚本、output/ 产出物、tmp/ 临时文件、cache/ 缓存）；" +
			"该目录已存在且对 Git 不可见。禁止在项目根新建 scripts / output / temp 之类的目录。",
		Priority: core.PriorityEssential,
	}}, nil
}

func (c *workspaceCap) Tools() []tool.Tool { return nil }

func (c *workspaceCap) Capture(_ context.Context, _ *CaptureCtx) error { return nil }
