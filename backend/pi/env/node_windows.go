//go:build windows

// Node 的安装根可能不在 PATH 上：官方安装包写注册表，装到别的盘也读得到。
package env

import "golang.org/x/sys/windows/registry"

// nodeRegistryDirs 读注册表里 Node 的安装目录。
func nodeRegistryDirs() []string {
	out := []string{}
	for _, path := range []string{`SOFTWARE\Node.js`, `SOFTWARE\WOW6432Node\Node.js`} {
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
