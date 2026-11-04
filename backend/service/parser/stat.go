package parser

import "os"

// FileInfo 是 os.FileInfo 的最小投影：只留我们真会用的两个字段。
type FileInfo struct {
	// SizeBytes 字节数
	SizeBytes int64
}

// Size 实现调用方需要的接口形状。
func (f FileInfo) Size() int64 { return f.SizeBytes }

// osStat 是 os.Stat 的一层薄封装：让 summary.go 里的错误分支保持可读。
func osStat(file string) (FileInfo, error) {
	info, err := os.Stat(file)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{SizeBytes: info.Size()}, nil
}
