// 技能加载与注册表链路：embed 加载（frontmatter 含列表也必须解析成功）→
// 按 id / 名字命中与来源优先级 → 内置技能落盘 → 切换工作目录换掉工作区技能。
package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// multiTypeFrontmatter 刻意混合标量、列表与行内数组，覆盖真实内置技能的写法。
const multiTypeFrontmatter = `---
name: office-docs
version: 1.0.0
when_to_use:
  - .docx
  - .xlsx
description: 在本机处理 Word / Excel / PPT / PDF。
allowed_tools: ["read", "write"]
---

# 正文
步骤一。
`

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"skills/office-docs/SKILL.md": &fstest.MapFile{Data: []byte(multiTypeFrontmatter)},
		"skills/plain/SKILL.md":       &fstest.MapFile{Data: []byte("---\nname: plain\ndescription: 简单技能。\n---\n\n正文\n")},
	}
}

func TestSkillRegistryChain(t *testing.T) {
	t.Run("embed 加载与正文取出", func(t *testing.T) {
		embedded := LoadFS(testFS(), "skills", domain.SkillSourceBuiltin)
		if len(embedded) != 2 {
			t.Fatalf("应加载 2 个内置技能，实际 %d", len(embedded))
		}
		// 带列表的 frontmatter 必须照样解析成功，否则技能被静默丢弃。
		byName := map[string]Embedded{}
		for _, e := range embedded {
			byName[e.Skill.Name] = e
		}
		off, ok := byName["office-docs"]
		if !ok {
			t.Fatal("含列表的 frontmatter 导致技能被跳过")
		}
		if off.Skill.Description == "" {
			t.Fatal("description 没有解析出来")
		}
		// 触发词必须解析并渲染给模型：写入 frontmatter 却没人读等于没写。
		if len(off.Skill.WhenToUse) != 2 {
			t.Fatalf("when_to_use 列表没解析出来: %+v", off.Skill.WhenToUse)
		}
		if off.Body == "" {
			t.Fatal("正文没有随技能一起取出")
		}

		// 内置技能走 InstallBuiltin：Location 必须落成磁盘上的真文件，
		// 否则模型按系统提示去 read 时读到的还是 embed 的虚拟路径。
		dir := t.TempDir()
		r := New()
		InstallBuiltin(r, testFS(), "skills", dir)
		got, ok := r.Get("office-docs")
		if !ok {
			t.Fatal("落盘后按名字取不到内置技能")
		}
		if !pkg.FileExists(got.Location) {
			t.Fatalf("内置技能没有落盘: %s", got.Location)
		}
		if want := filepath.Join(dir, "office-docs", "SKILL.md"); got.Location != want {
			t.Fatalf("落盘位置不对: 实际 %s，期望 %s", got.Location, want)
		}
		if raw, err := os.ReadFile(got.Location); err != nil || len(raw) == 0 {
			t.Fatalf("落盘的 SKILL.md 读不出来: err=%v", err)
		}
		if body, err := r.Content("office-docs"); err != nil || body == "" {
			t.Fatalf("内置技能正文取不到: err=%v body=%q", err, body)
		}
		// 渲染进系统提示的清单要带触发词，否则模型只能靠 description 猜场景。
		if render := r.Render(); !strings.Contains(render, "适用于：") {
			t.Fatalf("技能清单没有带触发词: %s", render)
		}
	})

	// 同名技能按来源优先级命中：工作区覆盖内置，撤掉工作区后回落到内置。
	t.Run("同名按来源优先且可回退", func(t *testing.T) {
		r := New()
		for _, e := range LoadFS(testFS(), "skills", domain.SkillSourceBuiltin) {
			r.AddWithBody(e.Skill, e.Body)
		}
		if _, ok := r.Get("plain"); !ok {
			t.Fatal("按名字取不到技能")
		}
		r.Add(domain.Skill{Name: "plain", Source: domain.SkillSourceWorkspace, Description: "w"})
		if got, ok := r.Get("plain"); !ok || got.Source != domain.SkillSourceWorkspace {
			t.Fatalf("同名时应返回优先级最高的来源，实际 %+v", got)
		}
		if !r.Remove("plain@workspace") {
			t.Fatal("按 id 删除失败")
		}
		if _, ok := r.Get("plain"); !ok {
			t.Fatal("删除 workspace 后应回落到内置技能")
		}
	})

	// 切换工作目录必须换掉工作区技能：留着上一个目录的那套，
	// 用户在新目录里会看到一堆叫不出名字、也打不开的技能。
	t.Run("切换工作目录换掉工作区技能", func(t *testing.T) {
		old, neu := t.TempDir(), t.TempDir()
		for _, ws := range []string{old, neu} {
			d := filepath.Join(ws, ".workbaby", "skills", "ws-skill")
			if err := pkg.EnsureDir(d); err != nil {
				t.Fatal(err)
			}
			if err := pkg.WriteText(filepath.Join(d, "SKILL.md"), "---\nname: ws-skill\ndescription: 工作区技能。\n---\n\n正文\n"); err != nil {
				t.Fatal(err)
			}
		}
		r := New()
		LoadWorkspace(r, old)
		sk, ok := r.Get("ws-skill")
		if !ok || sk.Source != domain.SkillSourceWorkspace {
			t.Fatalf("工作区技能没有装入: %+v", sk)
		}
		first := sk.Location

		LoadWorkspace(r, neu)
		sk, ok = r.Get("ws-skill")
		if !ok {
			t.Fatal("重载后技能丢了")
		}
		if sk.Location == first {
			t.Fatal("重载后仍指向上一个工作区的路径")
		}
		if n := len(r.List()); n != 1 {
			t.Fatalf("重载后应只剩当前工作区那一条，实际 %d", n)
		}

		LoadWorkspace(r, "")
		if len(r.List()) != 0 {
			t.Fatal("工作目录为空时应摘掉全部工作区技能")
		}
	})
}
