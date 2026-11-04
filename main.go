// 应用入口：装配 Wails 外壳并运行。业务与桌面层都在 backend/。
// 这里只做三件事：内嵌前端产物、读出构建期注入的版本号、把两者交给 shell.Run。
// 任何「启动逻辑」都不该长在这个文件里——它在根目录，最容易被当成放什么都行的抽屉。
package main

import (
	"embed"
	"os"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/shell"
)

//go:embed all:frontend/dist
var assets embed.FS

// version 由构建时注入（-ldflags "-X main.version=…"），跟着 git tag 走。
// 默认值是 dev：本地 go run 出来的就是开发版，不会误当成正式版。
var version = "dev"

func main() {
	if err := shell.Run(shell.Options{Version: version, Assets: assets}); err != nil {
		pkg.Error("app: 窗口启动失败: %v", err)
		os.Exit(1)
	}
}
