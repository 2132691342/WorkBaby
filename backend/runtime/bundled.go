// 运行时压缩包随二进制内嵌：用户不需要在 exe 旁边放任何文件。
// 归档与代码同仓库（Git LFS 托管，见 .gitattributes）；升级归档的同时必须
// 同步版本常量与 SHA 常量，三者不一致会被 TestBundledRuntimeChain 拦下。
package runtime

import (
	"bytes"
	"embed"

	"WorkBaby/backend/pkg"
)

//go:embed all:bundled
var bundledFS embed.FS

// lfsPointerPrefix 是未拉取的 LFS 文件的开头。构建机忘了 git lfs pull 时，
// 内嵌的就是这行 pointer 文本——当场识破，别把它当压缩包喂给解压器。
var lfsPointerPrefix = []byte("version https://git-lfs.github.com/spec/v1")

// archiveBytes 取内嵌归档并校验它真是一份压缩包。
func archiveBytes(name string) ([]byte, error) {
	data, err := bundledFS.ReadFile("bundled/" + name)
	if err != nil {
		return nil, pkg.New(7001, "运行时包未随构建嵌入：backend/runtime/bundled 下缺少 "+name, name)
	}
	if bytes.HasPrefix(data, lfsPointerPrefix) {
		return nil, pkg.New(7002, "运行时包是 Git LFS 指针：构建前先执行 git lfs pull", name)
	}
	return data, nil
}
