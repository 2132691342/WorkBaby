// 内置 Node 运行时：上游 win-x64 zip 随二进制内嵌，真正要用时才解压到数据目录。
// 归档含单层目录，解压必须剥掉它平铺到根，否则 npm.cmd 内部的相对路径指错。
package runtime

import (
	"os"
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

// NodeState 是内置 Node 的落盘现状：只读版本标记与占位，不解压。
// 系统里已有合规 node 时，一个字节都不该落到用户目录。
type NodeState struct {
	Dir     string
	Exe     string
	Version string
	Ready   bool
}

// InspectNode 读内置 Node 的现状，不触发解压。
func InspectNode(p Paths) NodeState {
	st := NodeState{Dir: p.NodeDir}
	if !nodeVersionMatches(p) {
		return st
	}
	exe := filepath.Join(p.NodeDir, "node.exe")
	if !pkg.FileExists(exe) {
		return st
	}
	st.Exe, st.Version, st.Ready = exe, nodeVersion, true
	return st
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
	// stripRoot 剥掉归档顶层那一个目录：Node 的 zip 与 MinGit 不同，
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
