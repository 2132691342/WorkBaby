// 系统能力的取用入口：当前只暴露用户主目录作为工作目录的缺省值。
package api

import "os"

// userHome 取用户主目录，作为工作目录的缺省值。
func userHome() (string, error) { return os.UserHomeDir() }
