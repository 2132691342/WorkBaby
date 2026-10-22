//go:build windows

// Git for Windows 可能装在任意盘符：注册表里的 InstallPath 是最可靠的线索。
package env

import "golang.org/x/sys/windows/registry"

// gitRegistryDirs 读注册表里 Git for Windows 的安装目录。
func gitRegistryDirs() []string {
	out := []string{}
	for _, path := range []string{
		`SOFTWARE\GitForWindows`,
		`SOFTWARE\WOW6432Node\GitForWindows`,
	} {
		for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
			key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			if dir, _, err := key.GetStringValue("InstallPath"); err == nil && dir != "" {
				out = append(out, dir)
			}
			key.Close()
		}
	}
	return out
}
