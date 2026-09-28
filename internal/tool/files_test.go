// 覆盖文件类工具的安全底线：路径穿越拒绝、Unicode 路径规整、写前必读、edit 唯一性、读后标记。
package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/internal/pkg"
)

// tracker 是测试用的已读记录。
type tracker struct{ seen map[string]bool }

func (t *tracker) HasRead(p string) bool { return t.seen[p] }
func (t *tracker) MarkRead(p string)     { t.seen[p] = true }

func newInput(t *testing.T, args map[string]any) (Input, string) {
	ws := t.TempDir()
	deps := Deps{Reads: &tracker{seen: map[string]bool{}}, TmpDir: ws}
	return Input{Args: args, Workspace: ws, Deps: deps}, ws
}

func TestReadRejectsPathTraversal(t *testing.T) {
	in, _ := newInput(t, map[string]any{"path": "../outside.txt"})
	if _, err := (readTool{}).Execute(context.Background(), in); err == nil {
		t.Fatal("工作目录外的路径必须被拒绝")
	} else if !strings.Contains(err.Error(), "1004") {
		t.Fatalf("期望路径穿越错误码 1004，实际 %v", err)
	}
}

func TestNormalizePath(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"不换行空格", "报表\u00a0数据.txt", "报表 数据.txt"},
		{"全角空格", "报表\u3000数据.txt", "报表 数据.txt"},
		{"零宽不换行空格", "报表\uFEFF数据.txt", "报表 数据.txt"},
		{"前导@", "@D:\\报表\\a.txt", "D:\\报表\\a.txt"},
		{"首尾空白", "  报表/数据.txt\t", "报表/数据.txt"},
		{"中文标点保留", "报表（2024）.xlsx", "报表（2024）.xlsx"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pkg.NormalizePath(c.in); got != c.want {
				t.Fatalf("NormalizePath(%q) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

func TestReadUnicodePathVariants(t *testing.T) {
	t.Run("复制来的不换行空格", func(t *testing.T) {
		in, ws := newInput(t, map[string]any{"path": "报表\u00a0数据.txt"})
		if err := pkg.WriteText(filepath.Join(ws, "报表 数据.txt"), "季度数据"); err != nil {
			t.Fatal(err)
		}
		res, err := (readTool{}).Execute(context.Background(), in)
		if err != nil {
			t.Fatalf("复制来的路径应能读到文件: %v", err)
		}
		if !strings.Contains(res.Content, "季度数据") {
			t.Fatalf("读到的内容不对: %q", res.Content)
		}
	})

	t.Run("路径里的直引号", func(t *testing.T) {
		// Windows 文件名里不允许直引号，磁盘上的引号只能是弯的。
		in, ws := newInput(t, map[string]any{"path": `"周报".txt`})
		if err := pkg.WriteText(filepath.Join(ws, "“周报”.txt"), "本周进展"); err != nil {
			t.Fatal(err)
		}
		if _, err := (readTool{}).Execute(context.Background(), in); err != nil {
			t.Fatalf("直引号变体应能找回弯引号命名的文件: %v", err)
		}
	})

	t.Run("确实不存在", func(t *testing.T) {
		in, _ := newInput(t, map[string]any{"path": "根本没有这个文件.xlsx"})
		_, err := (readTool{}).Execute(context.Background(), in)
		if err == nil {
			t.Fatal("读不存在的文件必须报错")
		}
		if pkg.CodeOf(err) != 1005 || !strings.Contains(err.Error(), "找不到这个文件") {
			t.Fatalf("期望明确的「找不到这个文件」错误，实际 %v", err)
		}
	})
}

func TestWriteRequiresReadFirst(t *testing.T) {
	in, ws := newInput(t, map[string]any{"path": "a.txt", "content": "hello"})
	target := filepath.Join(ws, "a.txt")
	if err := pkg.WriteText(target, "old"); err != nil {
		t.Fatal(err)
	}
	if _, err := (writeTool{}).Execute(context.Background(), in); err == nil {
		t.Fatal("写已存在但没读过的文件必须被拒绝")
	}
	// 读过之后应允许写入，回执要写明这是覆盖而不是新建。
	in.Deps.Reads.MarkRead(target)
	res, err := (writeTool{}).Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("读过后应允许写入: %v", err)
	}
	if !strings.Contains(res.Title, "覆盖了") {
		t.Fatalf("覆盖已有文件的回执必须写明覆盖: %q", res.Title)
	}
}

func TestEditSafety(t *testing.T) {
	t.Run("old_text 不唯一", func(t *testing.T) {
		in, ws := newInput(t, map[string]any{
			"path":  "b.txt",
			"edits": []any{map[string]any{"old_text": "重复", "new_text": "已改"}},
		})
		target := filepath.Join(ws, "b.txt")
		if err := pkg.WriteText(target, "重复\n重复\n"); err != nil {
			t.Fatal(err)
		}
		in.Deps.Reads.MarkRead(target)
		if _, err := (editTool{}).Execute(context.Background(), in); err == nil {
			t.Fatal("old_text 不唯一时必须报错")
		}
	})

	t.Run("old_text 等于 new_text", func(t *testing.T) {
		in, ws := newInput(t, map[string]any{
			"path":  "b2.txt",
			"edits": []any{map[string]any{"old_text": "原样", "new_text": "原样"}},
		})
		target := filepath.Join(ws, "b2.txt")
		if err := pkg.WriteText(target, "原样\n"); err != nil {
			t.Fatal(err)
		}
		in.Deps.Reads.MarkRead(target)
		if _, err := (editTool{}).Execute(context.Background(), in); err == nil {
			t.Fatal("没有实际改动的 edit 必须被拒绝")
		}
	})

	t.Run("保持行尾与 BOM", func(t *testing.T) {
		in, ws := newInput(t, map[string]any{
			"path":  "c.txt",
			"edits": []any{map[string]any{"old_text": "旧值", "new_text": "新值"}},
		})
		target := filepath.Join(ws, "c.txt")
		if err := os.WriteFile(target, []byte("\uFEFF第一行\r\n旧值\r\n第三行\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		in.Deps.Reads.MarkRead(target)
		if _, err := (editTool{}).Execute(context.Background(), in); err != nil {
			t.Fatalf("改文件失败: %v", err)
		}
		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if want := "\uFEFF第一行\r\n新值\r\n第三行\r\n"; string(got) != want {
			t.Fatalf("行尾或 BOM 被改动了，得到 %q", string(got))
		}
	})
}

func TestReadMarksFileAsRead(t *testing.T) {
	in, ws := newInput(t, map[string]any{"path": "d.txt"})
	target := filepath.Join(ws, "d.txt")
	if err := os.WriteFile(target, []byte("第一行\n第二行\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := (readTool{}).Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !strings.Contains(res.Content, "1\t") {
		t.Fatalf("输出应带行号: %q", res.Content)
	}
	if !in.Deps.Reads.HasRead(target) {
		t.Fatal("读过的文件必须被记录，供写前必读判断")
	}
}
