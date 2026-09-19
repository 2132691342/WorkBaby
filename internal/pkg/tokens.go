package pkg

// EstimateTextTokens 估算单段文本的 token 数：CJK 按 1 token/字，其余按 4 字符/token。
// 全项目唯一的文本估算实现，压缩阈值 / 上下文占用 / RAG 分块共用，各包不得就地另算。
func EstimateTextTokens(s string) int {
	if s == "" {
		return 0
	}
	cjk, other := 0, 0
	for _, r := range s {
		if isCJK(r) {
			cjk++
		} else {
			other++
		}
	}
	return cjk + (other+3)/4
}

// isCJK 判定中日韩字符：这些字符在主流分词器中近乎一字一 token，不能按 4 字符折算。
func isCJK(r rune) bool {
	return (r >= 0x3040 && r <= 0x30FF) || // 日文假名
		(r >= 0x3400 && r <= 0x4DBF) || // CJK 扩展 A
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK 基本区
		(r >= 0xF900 && r <= 0xFAFF) || // CJK 兼容表意文字
		(r >= 0xFF00 && r <= 0xFFEF) || // 全角 / 半角形式
		(r >= 0x3000 && r <= 0x303F) || // CJK 标点
		(r >= 0xAC00 && r <= 0xD7AF) // 韩文音节
}
