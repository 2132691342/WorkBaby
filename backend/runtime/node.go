// 内置 Node 运行时：上游 win-x64 zip 随二进制内嵌，首次启动解压到数据目录。
// 归档本身含单层目录（node-v24.21.0-win-x64/），必须剥掉它平铺到根——
// 不平铺则 npm.cmd 内部的相对路径会指错，表现为「npm 装好了但 PI 起不来」。
package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// nodeVersion 与归档在 bundled/ 下的文件名一一对应。
const (
	nodeVersion       = "24.21.0"
	nodeArchiveRooted = "node-v" + nodeVersion + "-win-x64"
)

// NodeArchiveSHA256 是内嵌归档的 SHA-256，换归档时同步更新；与 nodeVersion
// 必须同时改，否则版本标记命中会跳过解压，机器上永远用旧运行时。
const NodeArchiveSHA256 = "158f7685b44de51f6c0df1d153526cbcd3e1bc739a8dfc607721cef75de9e541"

// nodeArchiveName 返回内嵌归档文件名。
func nodeArchiveName() string { return nodeArchiveRooted + ".zip" }

// NodeExe 返回可用的 Node 路径；内置优先于系统。
// 内置优先是刻意的：PI 对 Node 版本有下界且行为随小版本变，
// 用用户 PATH 上某个 18.x 会让错误出现在协议层而不是启动检查里。
func NodeExe(p Paths) string {
	if exe, err := resolveNode(p); err == nil {
		return exe
	}
	return NodeSystemExe()
}

// resolveNode 只解析一次：并发调用共享同一个结果；缓存可被 ResetProbe 清空。
func resolveNode(p Paths) (string, error) {
	return probeOnce(&nodeProbed, &nodePath, &nodeErr, func() (string, error) {
		return EnsureNode(p)
	})
}

// NodeStatus 报告内置 Node 的可用状态与失败原因。
type NodeStatus struct {
	Exe     string
	Err     string
	Source  string
	Version string
}

// NodeStatusOf 探测内置 Node。
func NodeStatusOf(p Paths) NodeStatus {
	exe, err := EnsureNode(p)
	if err != nil {
		return NodeStatus{Err: err.Error()}
	}
	return NodeStatus{Exe: exe, Source: "bundled", Version: nodeVersion}
}

// NpmCmd 返回内置 npm 的入口（Windows 上是 npm.cmd）。
func NpmCmd(p Paths) string {
	for _, name := range []string{"npm.cmd", "npm"} {
		candidate := filepath.Join(p.NodeDir, name)
		if pkg.FileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// EnsureNode 保证内置 Node 已解压并返回 node 的路径。
func EnsureNode(p Paths) (string, error) {
	exe := filepath.Join(p.NodeDir, "node.exe")
	if nodeVersionMatches(p) && pkg.FileExists(exe) {
		return exe, nil
	}
	data, err := archiveBytes(nodeArchiveName())
	if err != nil {
		return "", err
	}
	if err := dropStaleRuntime(p.NodeDir, nodeVersion); err != nil {
		return "", err
	}
	reader, err := newZipReader(data)
	if err != nil {
		return "", err
	}
	// stripRoot 剥掉归档顶层那一个目录：Node 的 zip 与 Python / PowerShell 不同，
	// 它不是平铺的，不剥层会让 node.exe 多一层路径。
	if err := extractZipStripped(reader, p.NodeDir, nodeArchiveRooted); err != nil {
		return "", err
	}
	if err := pkg.WriteText(filepath.Join(p.NodeDir, ".version"), nodeVersion); err != nil {
		return "", err
	}
	if !pkg.FileExists(exe) {
		return "", pkg.New("7001", "内置 Node 解压后找不到 node.exe", exe)
	}
	return exe, nil
}

// nodeVersionMatches 比对版本标记，一致就跳过解压。
func nodeVersionMatches(p Paths) bool {
	raw, err := os.ReadFile(filepath.Join(p.NodeDir, ".version"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == nodeVersion
}

// NodeSystemExe 返回系统 PATH 上的 node（内置不可用时兜底）。
func NodeSystemExe() string {
	for _, name := range []string{"node.exe", "node"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe
		}
	}
	return ""
}
