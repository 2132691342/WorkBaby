package pkg

import (
	"path/filepath"
	"strings"
)

// NormalizePath 规整用户从聊天/网页/Word 复制来的路径：
// 不换行空格、全角空格、零宽空格一律转成普通空格，剥掉前导 '@'。
func NormalizePath(p string) string {
	s := strings.TrimSpace(foldSpaces(p))
	return strings.TrimSpace(strings.TrimPrefix(s, "@"))
}

// foldSpaces 把「看起来像空格」的码点换成普通空格：这些码点肉眼不可见，
// 系统却不会当空格处理，规整之后复制来的路径才能直接用。
func foldSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if spaceLike(r) {
			return ' '
		}
		return r
	}, s)
}

// spaceLike 覆盖复制粘贴最常带进来的伪装空格：U+00A0 不换行空格、
// U+2000-U+200A、U+202F 窄不换行空格、U+205F 中型空格、U+3000 全角空格、U+FEFF 零宽不换行空格。
func spaceLike(r rune) bool {
	switch {
	case r == 0x00A0, r == 0x202F, r == 0x205F, r == 0x3000, r == 0xFEFF:
		return true
	case r >= 0x2000 && r <= 0x200A:
		return true
	}
	return false
}

// SafeJoin 把用户给出的路径限制在 root 内：解析真实路径后必须仍在 root 之下。
// 符号链接与 .. 都会在 EvalSymlinks 后暴露，因此这是路径穿越的唯一判据。
func SafeJoin(root, target string) (string, error) {
	root = filepath.Clean(root)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", Wrap(1003, "工作目录解析失败", err)
	}
	full := NormalizePath(target)
	if !filepath.IsAbs(full) {
		full = filepath.Join(absRoot, full)
	}
	full = filepath.Clean(full)

	realRoot, err := resolveReal(absRoot)
	if err != nil {
		return "", err
	}
	realFull, err := resolveReal(full)
	if err != nil {
		return "", err
	}
	if !IsInside(realRoot, realFull) {
		return "", New(1004, "不能操作工作目录之外的路径", realFull)
	}
	return realFull, nil
}

// resolveReal 解析真实路径。目标尚不存在（新建文件）时，
// 解析其最深的存在祖先再拼接剩余部分，避免绕过 EvalSymlinks 放过符号链接穿越。
func resolveReal(p string) (string, error) {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real, nil
	}
	cleaned := filepath.Clean(p)
	base := cleaned
	var rest []string
	for {
		parent := filepath.Dir(base)
		if parent == base {
			return cleaned, nil
		}
		rest = append([]string{filepath.Base(base)}, rest...)
		if real, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		base = parent
	}
}

// IsInside 判断 child 是否等于 root 或位于其下（大小写不敏感，适配 Windows）。
func IsInside(root, child string) bool {
	root = filepath.Clean(strings.ToLower(root))
	child = filepath.Clean(strings.ToLower(child))
	if root == child {
		return true
	}
	return strings.HasPrefix(child, root+string(filepath.Separator))
}

// RelPath 返回相对 root 的展示路径；失败时回退原值。
func RelPath(root, full string) string {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return full
	}
	return rel
}
