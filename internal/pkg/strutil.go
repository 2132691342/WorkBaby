package pkg

// TruncateRunes 按 rune 截断；n<=0 或长度未超限时原样返回，超限追加省略号。
func TruncateRunes(s string, n int) string {
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
