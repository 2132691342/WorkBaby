package platform

// OpenPath 用系统默认程序打开一个路径。
func OpenPath(path string) error { return openPath(trimsQuotes(path)) }

// RevealPath 在资源管理器里定位一个路径。
func RevealPath(path string) error { return revealPath(trimsQuotes(path)) }
