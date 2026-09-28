// 覆盖技能加载的三处静默失效：
// ① embed 目录传错 → 内置技能一个都加载不到；
// ② frontmatter 里有列表/布尔 → map[string]string 解析失败 → 技能被跳过；
// ③ 内置技能的 Location 是虚拟路径，正文必须从内存取，否则 read 打不开。

package skill

import (
	"testing"
	"testing/fstest"

	"WorkBaby/internal/domain"
)

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

func TestLoadFSReadsEmbedRoot(t *testing.T) {
	got := LoadFS(testFS(), "skills", domain.SkillSourceBuiltin)
	if len(got) != 2 {
		t.Fatalf("应加载 2 个内置技能，实际 %d", len(got))
	}
	// 带列表的 frontmatter 必须照样解析成功，否则技能被静默丢弃。
	byName := map[string]Embedded{}
	for _, e := range got {
		byName[e.Skill.Name] = e
	}
	off, ok := byName["office-docs"]
	if !ok {
		t.Fatal("含列表的 frontmatter 导致技能被跳过")
	}
	if off.Skill.Description == "" {
		t.Fatal("description 没有解析出来")
	}
	if off.Body == "" || off.Body[0] == '#' && off.Skill.Name == "" {
		t.Fatal("正文没有随技能一起取出")
	}
}

func TestContentServesEmbeddedBodyWithoutDisk(t *testing.T) {
	r := New()
	for _, e := range LoadFS(testFS(), "skills", domain.SkillSourceBuiltin) {
		r.AddWithBody(e.Skill, e.Body)
	}
	// Location 指向 embed 里的虚拟路径，磁盘上根本没有这个文件。
	body, err := r.Content("office-docs")
	if err != nil {
		t.Fatalf("内置技能正文取不到: %v", err)
	}
	if body == "" {
		t.Fatal("正文为空")
	}
}

func TestGetAcceptsBothIdAndName(t *testing.T) {
	r := New()
	for _, e := range LoadFS(testFS(), "skills", domain.SkillSourceBuiltin) {
		r.AddWithBody(e.Skill, e.Body)
	}
	// 前端拿到的是 id（name@source），后端也要能按 id 命中，否则删除这类操作永远落空
	if _, ok := r.Get("plain@builtin"); !ok {
		t.Fatal("按 id 取不到技能")
	}
	if _, ok := r.Get("plain"); !ok {
		t.Fatal("按名字取不到技能")
	}
	// 同名不同源：workspace 优先级最高
	r.Add(domain.Skill{Name: "plain", Source: domain.SkillSourceWorkspace, Description: "w"})
	got, ok := r.Get("plain")
	if !ok || got.Source != domain.SkillSourceWorkspace {
		t.Fatalf("同名时应返回优先级最高的来源，实际 %+v", got)
	}
	// 删除按 id 也要能移除干净
	if !r.Remove("plain@workspace") {
		t.Fatal("按 id 删除失败")
	}
	if _, ok := r.Get("plain@workspace"); ok {
		t.Fatal("删除后仍在注册表里")
	}
}

func TestRegistryReaddAndListAreSafe(t *testing.T) {
	r := New()
	for _, e := range LoadFS(testFS(), "skills", domain.SkillSourceBuiltin) {
		r.AddWithBody(e.Skill, e.Body)
	}
	// 读锁配对错误会让这里直接 panic；反复调用确认不会。
	for i := 0; i < 3; i++ {
		if len(r.List()) != 2 {
			t.Fatal("列表数量不对")
		}
		if _, ok := r.Get("plain"); !ok {
			t.Fatal("按名取技能失败")
		}
	}
	// 同名覆盖不应让 order 出现重复项。
	r.AddWithBody(domain.Skill{Name: "plain", Source: domain.SkillSourceWorkspace, Description: "x"}, "y")
	if len(r.List()) != 3 {
		t.Fatalf("覆盖后应仍是 3 条，实际 %d", len(r.List()))
	}
}
