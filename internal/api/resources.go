package api

import (
	"io/fs"
	"os"

	"WorkBaby/assets"
)

// builtinSkills 返回内置技能的嵌入文件系统。
func builtinSkills() fs.FS { return assets.Skills }

// userHome 取用户主目录，作为工作目录的缺省值。
func userHome() (string, error) { return os.UserHomeDir() }
