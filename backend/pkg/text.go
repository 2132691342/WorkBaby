// 文本解码：Windows 上同一份「文本」可能是 UTF-8、GBK 或 UTF-16，
// 一律按 UTF-8 读会把记事本「Unicode」与 PowerShell 重定向的输出读成乱码。
package pkg

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// DecodeText 把一段字节归一成 UTF-8 文本。判定顺序：BOM → UTF-16 启发式 → GBK。
func DecodeText(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// BOM 是声明式的，不会猜错，最优先。
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return decodeUTF16(b[2:], false)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return decodeUTF16(b[2:], true)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if bigEndian, ok := looksUTF16(b); ok {
		return decodeUTF16(b, bigEndian)
	}
	out, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

// LooksBinary 判断字节流是否二进制：UTF-16 文本在字节层面也含大量 NUL，
// 必须先按文本形态排除，再找控制字符，否则记事本存的 txt 会被判成二进制。
func LooksBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if _, ok := looksUTF16(b); ok || bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		return false
	}
	n := len(b)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}

func decodeUTF16(b []byte, bigEndian bool) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return string(utf16.Decode(u))
}

// looksUTF16 判定无 BOM 的 UTF-16：抽样看奇数 / 偶数位 NUL 的占比。
// LE 的 ASCII 文本奇数位几乎全是 NUL，BE 相反；两边都高说明不是文本。
func looksUTF16(b []byte) (bigEndian bool, ok bool) {
	n := len(b)
	if n < 8 {
		return false, false
	}
	if n > 512 {
		n = 512
	}
	zeroOdd, zeroEven := 0, 0
	for i := 0; i < n; i++ {
		if b[i] != 0 {
			continue
		}
		if i%2 == 0 {
			zeroEven++
		} else {
			zeroOdd++
		}
	}
	quarter := n / 4
	switch {
	case zeroOdd > quarter && zeroEven < zeroOdd/4:
		return false, true
	case zeroEven > quarter && zeroOdd < zeroEven/4:
		return true, true
	}
	return false, false
}
