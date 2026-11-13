package pkg

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxSessionBytes 是单份会话文件的读取上限（64MB）。
// 工具输出会把文件堆得很大，整读再解析遇到几百 MB 的会话会把应用拖死；
// 超限读后半段——界面要的是最近的活动分支，早期的轮次本来也翻不到。
const maxSessionBytes = 64 << 20

// ReadSessionLines 读会话文件并按行切开，跳过超限部分的前半段。
// 引擎要读回自己的会话、回放层要读同一份文件，两边共用这一处：
// 上限与切行规则只在这里定义一次，否则两边迟早读出不一样的结果。
func ReadSessionLines(file string) ([]string, error) {
	raw, err := readCapped(file, maxSessionBytes)
	if err != nil {
		return nil, err
	}
	return SplitLines(string(raw)), nil
}

// readCapped 最多读 limit 字节；超限时保留后半段。
// 会话文件的尾巴才是最近一轮，读开头反而丢掉用户最想看的那段。
func readCapped(file string, limit int64) ([]byte, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	if info.Size() <= limit {
		return os.ReadFile(file)
	}
	handle, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	if _, err := handle.Seek(info.Size()-limit, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(handle, limit))
	if err != nil {
		return nil, err
	}
	// 从中间切进来时首行多半是半截 JSON：丢掉这一行，别把它当坏行以外的特殊情形。
	if idx := bytes.IndexByte(raw, '\n'); idx >= 0 {
		raw = raw[idx+1:]
	}
	return raw, nil
}

// EncodedWorkspace 把工作区路径编码成会话目录名：剥掉头部分隔符，/ \ : 空格换 -，两端包 --。
// 引擎建目录与清单扫目录共用这一个函数，两边各写一套会把新会话落进清单扫不到的目录。
func EncodedWorkspace(workspace string) string {
	stripped := strings.TrimLeft(workspace, `\/`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-", " ", "-")
	return "--" + replacer.Replace(stripped) + "--"
}

// SessionDirOf 取一个工作区对应的会话目录。
func SessionDirOf(sessionsRoot, workspace string) string {
	return filepath.Join(sessionsRoot, EncodedWorkspace(workspace))
}
