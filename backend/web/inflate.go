package web

import (
	"bytes"
	"compress/zlib"
	"io"
)

// inflate 解一段 zlib 压缩数据。
// PDF 的 FlateDecode 流用的就是 zlib 包装。
// 解不开时给 false：调用方会保留原样，而不是丢掉整段内容。
func inflate(data []byte) ([]byte, bool) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	defer reader.Close()
	// 限长：解压炸弹能撑爆内存，PDF 流解压后不该有几兆。
	out, err := io.ReadAll(io.LimitReader(reader, maxInflatedBytes))
	if err != nil && len(out) == 0 {
		return nil, false
	}
	return out, true
}

// maxInflatedBytes 是单段解压后的上限。
const maxInflatedBytes = 32 << 20
