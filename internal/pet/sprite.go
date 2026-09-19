package pet

// DetectExt 凭文件头 magic number 推断图片真实格式（防扩展名伪装）。
// 返回 ".png" / ".jpg" / ".gif" / ".webp"；无法识别返回空串。
func DetectExt(data []byte) string {
	switch {
	case len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return ".png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return ".jpg"
	case len(data) >= 6 && data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8':
		return ".gif"
	case len(data) >= 12 && data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P':
		return ".webp"
	}
	return ""
}
