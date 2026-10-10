// 技能注册表链路：embed 解析（frontmatter 含列表也必须成功）→ 落盘成真文件 → 渲染进提示，
// 以及同名来源优先级与切换工作目录。
// 坏了的表现：技能被静默丢弃、模型读了虚拟路径读不到正文、旧工作区的技能残留。
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
	// 从 embed 解析到落盘：Location 必须是磁盘上的真文件，
	// 否则模型按系统提示去 read 时读到的还是 embed 的虚拟路径。
	t.Run("embed 解析、落盘与渲染", func(t *testing.T) {
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
		if off.Skill.Description == "" || off.Body == "" {
			t.Fatalf("描述或正文没有解析出来: %+v", off.Skill)
		}
		// 触发词必须解析并渲染给模型：写入 frontmatter 却没人读等于没写。
		if len(off.Skill.WhenToUse) != 2 {
			t.Fatalf("when_to_use 列表没解析出来: %+v", off.Skill.WhenToUse)
		}

		dir := t.TempDir()
		r := New()
		InstallBuiltin(r, testFS(), "skills", dir)
		got, ok := r.Get("office-docs")
		if !ok || !pkg.FileExists(got.Location) {
			t.Fatalf("内置技能没有落盘: %+v", got)
		}
		if want := filepath.Join(dir, "office-docs", "SKILL.md"); got.Location != want {
			t.Fatalf("落盘位置不对: 实际 %s，期望 %s", got.Location, want)
		}
		if raw, err := os.ReadFile(got.Location); err != nil || len(raw) == 0 {
			t.Fatalf("落盘的 SKILL.md 读不出来: err=%v", err)
		}
		if body, err := r.Content("office-docs"); err != nil || body == "" {
			t.Fatalf("正文取不到: err=%v body=%q", err, body)
		}
		if render := r.Render(); !strings.Contains(render, "适用于：") {
			t.Fatalf("技能清单没有带触发词: %s", render)
		}
	})

	// 同名技能按来源优先级命中（工作区覆盖内置，撤掉后回落），
	// 切换工作目录必须换掉工作区技能：留着上一个目录那套，用户会看到一堆打不开的技能。
	t.Run("来源优先级与工作区切换", func(t *testing.T) {
		r := New()
		for _, e := range LoadFS(testFS(), "skills", domain.SkillSourceBuiltin) {
			r.AddWithBody(e.Skill, e.Body)
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
		LoadWorkspace(r, old)
		sk, ok := r.Get("ws-skill")
		if !ok || sk.Source != domain.SkillSourceWorkspace {
			t.Fatalf("工作区技能没有装入: %+v", sk)
		}
		first := sk.Location

		LoadWorkspace(r, neu)
		sk, ok = r.Get("ws-skill")
		if !ok || sk.Location == first {
			t.Fatalf("重载后仍指向上一个工作区: %+v", sk)
		}
		// 重载只换工作区那部分：2 个内置技能一条都不能少，工作区技能换成新目录那份。
		if n := len(r.List()); n != 3 {
			t.Fatalf("重载后应是 2 个内置技能 + 1 个工作区技能，实际 %d", n)
		}

		LoadWorkspace(r, "")
		if _, ok := r.Get("ws-skill"); ok {
			t.Fatal("工作目录为空时应摘掉工作区技能")
		}
		if _, ok := r.Get("plain"); !ok {
			t.Fatal("内置技能不该被工作区重载清掉")
		}
	})
}
