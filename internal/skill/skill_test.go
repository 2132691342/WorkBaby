// 技能加载与注册表的三处静默失效：
// ① embed 根路径传错 → 内置技能一个都加载不到；
// ② frontmatter 含列表 / 布尔 → 解析失败 → 技能被静默跳过；
// ③ 内置技能 Location 是虚拟路径，正文必须从内存取，否则 read 打不开。
package skill

import (
	"testing"
	"testing/fstest"

	"WorkBaby/internal/domain"
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

// loaded 是「加载到内存」与「按 id / 名字取到正文」的合并链路：
// 加载失败、正文取不到、优先级选错，任意一处坏掉前端技能页就空着。
func TestEmbeddedSkillsLoadAndResolve(t *testing.T) {
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
	if off.Body == "" {
		t.Fatal("正文没有随技能一起取出")
	}

	r := New()
	for _, e := range embedded {
		r.AddWithBody(e.Skill, e.Body)
	}
	// 内置技能的 Location 指向 embed 里的虚拟路径，磁盘上根本没有这个文件。
	if body, err := r.Content("office-docs"); err != nil || body == "" {
		t.Fatalf("内置技能正文取不到: err=%v body=%q", err, body)
	}
}

// 注册表按 id（name@source）与名字两种方式命中；同名时 workspace 覆盖 builtin。
// 前端拿到的是 id，后端若只认名字，删除一类操作会永远落空。
func TestRegistryLookupPrecedenceAndRemoval(t *testing.T) {
	r := New()
	for _, e := range LoadFS(testFS(), "skills", domain.SkillSourceBuiltin) {
		r.AddWithBody(e.Skill, e.Body)
	}
	if _, ok := r.Get("plain@builtin"); !ok {
		t.Fatal("按 id 取不到技能")
	}
	if _, ok := r.Get("plain"); !ok {
		t.Fatal("按名字取不到技能")
	}

	r.Add(domain.Skill{Name: "plain", Source: domain.SkillSourceWorkspace, Description: "w"})
	got, ok := r.Get("plain")
	if !ok || got.Source != domain.SkillSourceWorkspace {
		t.Fatalf("同名时应返回优先级最高的来源，实际 %+v", got)
	}
	if !r.Remove("plain@workspace") {
		t.Fatal("按 id 删除失败")
	}
	if _, ok := r.Get("plain@workspace"); ok {
		t.Fatal("删除后仍在注册表里")
	}
	// 覆盖回内置来源后仍应命中，证明删除不是把整个名字一起抹掉。
	if _, ok := r.Get("plain"); !ok {
		t.Fatal("删除 workspace 后应回落到内置技能")
	}
}

// 读锁配对错误（配成 Unlock）会让这里直接 panic；同一 id 重复登记不应让列表出现重复项。
func TestRegistryReadLockPairingAndOverwrite(t *testing.T) {
	r := New()
	for _, e := range LoadFS(testFS(), "skills", domain.SkillSourceBuiltin) {
		r.AddWithBody(e.Skill, e.Body)
	}
	for i := 0; i < 3; i++ {
		if len(r.List()) != 2 {
			t.Fatal("列表数量不对")
		}
		if _, ok := r.Get("plain"); !ok {
			t.Fatal("按名取技能失败")
		}
	}
	// 同一 id 再登记一次：替换而非追加。
	again := domain.Skill{Name: "plain", Source: domain.SkillSourceBuiltin, Description: "x"}
	r.AddWithBody(again, "y")
	if len(r.List()) != 2 {
		t.Fatalf("同一 id 重复登记后应仍是 2 条，实际 %d", len(r.List()))
	}
	// 不同来源是两个独立条目，同名时由 Get 按优先级选。
	r.AddWithBody(domain.Skill{Name: "plain", Source: domain.SkillSourceWorkspace, Description: "x"}, "y")
	if len(r.List()) != 3 {
		t.Fatalf("不同来源应各占一条，实际 %d", len(r.List()))
	}
}
