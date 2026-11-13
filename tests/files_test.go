// 文件预览：文本 / 可预览二进制 / 不可预览二进制的分类边界。
package tests

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// TestFilePreview 断言文件预览的三条分类边界：文本回正文、可预览二进制回 base64、
// 认不出的二进制只给占位。这三条是「界面能不能看」的唯一判据，改坏了预览就白屏。
func TestFilePreview(t *testing.T) {
	h := newHarness(t)
	ws := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatalf("准备工作区失败：%v", err)
	}
	if err := h.app.SetWorkspace(ws); err != nil {
		t.Fatalf("绑定工作区失败：%v", err)
	}

	// ---- 文本：回正文，不算二进制 ----
	textBody := "你好，WorkBaby\n第二行"
	if err := os.WriteFile(filepath.Join(ws, "note.txt"), []byte(textBody), 0o644); err != nil {
		t.Fatalf("写文本文件失败：%v", err)
	}
	out, err := h.app.Files.Read("note.txt")
	if err != nil {
		t.Fatalf("读文本文件失败：%v", err)
	}
	if out.IsBinary || out.FileKind != "text" {
		t.Errorf("txt 应是文本：isBinary=%v kind=%q", out.IsBinary, out.FileKind)
	}
	if out.FileText != textBody {
		t.Errorf("文本应回正文：期望 %q 实际 %q", textBody, out.FileText)
	}

	// ---- 可预览二进制（png）：回 base64 与 image 类型 ----
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0x0D, 0x0A, 0x1A, 0x0A, 0, 1, 2, 3}
	if err := os.WriteFile(filepath.Join(ws, "pic.png"), pngBytes, 0o644); err != nil {
		t.Fatalf("写图片失败：%v", err)
	}
	out, err = h.app.Files.Read("pic.png")
	if err != nil {
		t.Fatalf("读图片失败：%v", err)
	}
	if !out.IsBinary || out.FileKind != "image" {
		t.Errorf("png 应是可预览二进制：isBinary=%v kind=%q", out.IsBinary, out.FileKind)
	}
	decoded, decErr := base64.StdEncoding.DecodeString(out.FileBase64)
	if decErr != nil {
		t.Fatalf("base64 解码失败：%v", decErr)
	}
	if string(decoded) != string(pngBytes) {
		t.Errorf("base64 应还原成原始字节：期望 %v 实际 %v", pngBytes, decoded)
	}

	// ---- 认不出的二进制：只给占位，不回 base64 ----
	binBytes := []byte{0, 1, 2, 3, 0, 4, 5}
	if err := os.WriteFile(filepath.Join(ws, "blob.dat"), binBytes, 0o644); err != nil {
		t.Fatalf("写二进制失败：%v", err)
	}
	out, err = h.app.Files.Read("blob.dat")
	if err != nil {
		t.Fatalf("读二进制失败：%v", err)
	}
	if !out.IsBinary || out.FileKind != "binary" {
		t.Errorf("dat 应是不预览的二进制：isBinary=%v kind=%q", out.IsBinary, out.FileKind)
	}
	if out.FileBase64 != "" {
		t.Errorf("认不出的二进制不该回 base64，实际 %d 字节", len(out.FileBase64))
	}
}
