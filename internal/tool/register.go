package tool

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
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}
