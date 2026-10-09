package tool

import "WorkBaby/backend/domain"

// toolCategory 是内置工具的展示分类。集中声明在这里而不是散落在各工具里：
// 分类只服务于「工具」菜单的分组，与工具实现正交，改分类不必碰执行逻辑。
var toolCategory = map[string]string{
	"read":             domain.CategoryFile,
	"write":            domain.CategoryFile,
	"edit":             domain.CategoryFile,
	"ls":               domain.CategoryFile,
	"find":             domain.CategoryFile,
	"grep":             domain.CategoryFile,
	"powershell":       domain.CategoryShell,
	"python":           domain.CategoryCode,
	"web_search":       domain.CategoryWeb,
	"web_fetch":        domain.CategoryWeb,
	"knowledge_search": domain.CategoryData,
}

// Category 实现 Categorized：按注册时的映射表返回，未登记的走兜底类。
func categorized(t Tool) Tool { return &withCategory{Tool: t, cat: toolCategory[t.Name()]} }

// withCategory 只覆盖分类，其余方法原样转发。
type withCategory struct {
	Tool
	cat string
}

func (w *withCategory) Category() string { return w.cat }

// RegisterBuiltins 是内置工具的唯一注册入口。
func RegisterBuiltins(r *Registry) error {
	for _, t := range []Tool{
		readTool{},
		writeTool{},
		editTool{},
		lsTool{},
		findTool{},
		grepTool{},
		powershellTool{},
		pythonTool{},
		webSearchTool{},
		webFetchTool{},
	} {
		if err := r.Register(categorized(t)); err != nil {
			return err
		}
	}
	return nil
}
