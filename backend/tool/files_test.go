// 文件类工具的安全底线：路径穿越拒绝、Unicode 路径规整与找回、写前必读、edit 唯一性与行尾保持。
// 这些护栏一旦失效，模型会写错文件或写到工作目录外，属于不可逆后果。
package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/pkg"
)

// readTracker 记录已读文件，供写前必读判断。
type readTracker struct{ seen map[string]bool }

func (t *readTracker) HasRead(p string) bool { return t.seen[p] }
func (t *readTracker) MarkRead(p string)     { t.seen[p] = true }

func newInput(t *testing.T, args map[string]any) (Input, string) {
	t.Helper()
	ws := t.TempDir()
	return Input{Args: args, Workspace: ws, Deps: Deps{Reads: &readTracker{seen: map[string]bool{}}, TmpDir: ws}}, ws
}

// 文件类工具的完整护栏链路：读（穿越拒绝、Unicode 找回）与写改（写前必读、edit 唯一性、行尾保持）。
func TestFilesGuardrails(t *testing.T) {
	t.Run("路径穿越被拒绝", func(t *testing.T) {
		in, _ := newInput(t, map[string]any{"path": "../outside.txt"})
		_, err := (readTool{}).Execute(context.Background(), in)
		if err == nil {
			t.Fatal("工作目录外的路径必须被拒绝")
		}
		if pkg.CodeOf(err) != 1004 {
			t.Fatalf("期望路径穿越错误码 1004，实际 %v", err)
		}
	})

	// 模型拿到的路径常带不可见字符与全角标点，读工具要能规整后命中磁盘上的真实文件。
	t.Run("Unicode 路径变体找回", func(t *testing.T) {
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

		// Windows 文件名里不允许直引号，磁盘上的引号只能是弯的。
		in, ws = newInput(t, map[string]any{"path": `"周报".txt`})
		if err := pkg.WriteText(filepath.Join(ws, "“周报”.txt"), "本周进展"); err != nil {
			t.Fatal(err)
		}
		if _, err := (readTool{}).Execute(context.Background(), in); err != nil {
			t.Fatalf("直引号变体应能找回弯引号命名的文件: %v", err)
		}
	})

	// 读→写闭环：没读过就写必须被拒；read 工具真读一次即完成记账，write 随即放行。
	// 记账这一步在 read 工具内部，写工具的拒绝分支不覆盖它——所以必须走真的读一次。
	t.Run("覆盖前必读", func(t *testing.T) {
		ws := t.TempDir()
		deps := Deps{Reads: &readTracker{seen: map[string]bool{}}, TmpDir: ws}
		target := filepath.Join(ws, "a.txt")
		if err := pkg.WriteText(target, "old"); err != nil {
			t.Fatal(err)
		}
		wIn := Input{Args: map[string]any{"path": "a.txt", "content": "hello"}, Workspace: ws, Deps: deps}
		if _, err := (writeTool{}).Execute(context.Background(), wIn); err == nil {
			t.Fatal("写已存在但没读过的文件必须被拒绝")
		}
		rIn := Input{Args: map[string]any{"path": "a.txt"}, Workspace: ws, Deps: deps}
		if _, err := (readTool{}).Execute(context.Background(), rIn); err != nil {
			t.Fatalf("读取失败: %v", err)
		}
		res, err := (writeTool{}).Execute(context.Background(), wIn)
		if err != nil {
			t.Fatalf("读过之后应允许写入（记账失效会让这里失败）: %v", err)
		}
		if !strings.Contains(res.Title, "覆盖了") {
			t.Fatalf("覆盖已有文件的回执必须写明覆盖: %q", res.Title)
		}
	})

	t.Run("edit 唯一性与实际改动", func(t *testing.T) {
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

		in, ws = newInput(t, map[string]any{
			"path":  "b2.txt",
			"edits": []any{map[string]any{"old_text": "原样", "new_text": "原样"}},
		})
		target = filepath.Join(ws, "b2.txt")
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
