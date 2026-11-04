package shell

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// Endpoint 是回填给前端的端点信息。
type Endpoint struct {
	// Port 本地服务监听端口
	Port int `json:"wb_port"`
	// Token 启动令牌，写进每个请求的 X-Desktop-Token
	Token string `json:"wb_token"`
	// Version 应用版本
	Version string `json:"wb_version"`
	// Contract 响应契约版本
	Contract int `json:"wb_contract"`
	// BaseURL 拼好的基地址，前端直接给 axios
	BaseURL string `json:"wb_base_url"`
}

// filepathDir 取文件所在目录。
func filepathDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Dir(abs), nil
}

// layoutDataSub 拼数据目录下的子目录。
func layoutDataSub(layout pkg.Layout, name string) string {
	return filepath.Join(layout.DataDir, name)
}

// findRepoRoot 从当前工作目录往上找带 go.mod 的目录。
// 开发态的 exe 在临时目录里跑，只有靠这个才能把资产指回仓库根。
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 8; i++ {
		if pkg.Exists(filepath.Join(dir, "go.mod")) && pkg.IsDir(filepath.Join(dir, "assets")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || strings.TrimSpace(parent) == "" {
			return ""
		}
		dir = parent
	}
	return ""
}
