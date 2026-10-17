// 技能注册表链路：两区目录扫描 → 列表与正文，以及来源优先级与工作区切换。
// 坏了的表现：技能被静默丢弃、停用的技能混进启用清单、旧工作区的技能残留。
package skill

import (
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// multiTypeFrontmatter 刻意混合标量、列表与行内数组，覆盖手写技能的常见写法。
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

// writeSkill 在 dir 下写一个技能，返回它的 SKILL.md 路径。
func writeSkill(t *testing.T, dir, name, body string) string {
	t.Helper()
	d := filepath.Join(dir, name)
	if err := pkg.EnsureDir(d); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, "SKILL.md")
	if err := pkg.WriteText(p, body); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSkillRegistryChain(t *testing.T) {
	// frontmatter 混合类型必须照样解析：解析失败会被静默跳过，用户看不到任何提示。
	t.Run("两区扫描、正文与触发词", func(t *testing.T) {
		dir := t.TempDir()
		writeSkill(t, dir, "office-docs", multiTypeFrontmatter)
		writeSkill(t, dir, "plain", "---\nname: plain\ndescription: 简单技能。\n---\n\n正文\n")
		disabled := filepath.Join(t.TempDir(), "skills.disabled")
		writeSkill(t, disabled, "paused", "---\nname: paused\ndescription: 停用中的技能。\n---\n\n正文\n")

		r := New()
		for _, s := range LoadDir(dir, domain.SkillSourceGlobal) {
			r.Add(s)
		}
		for _, s := range LoadDisabledDir(disabled, domain.SkillSourceGlobal) {
			r.Add(s)
		}
		if n := len(r.List()); n != 3 {
			t.Fatalf("应装入 3 个技能（2 启用 + 1 停用），实际 %d", n)
		}
		off, ok := r.Get("office-docs")
		if !ok || off.Description == "" || !off.Enabled {
			t.Fatalf("启用技能没有装进来: %+v", off)
		}
		// 触发词解析出来界面才能展示「什么时候会用它」。
		if len(off.WhenToUse) != 2 {
			t.Fatalf("when_to_use 列表没解析出来: %+v", off.WhenToUse)
		}
		// 停用的技能也在列表里、正文可读——界面要列得出、能一键恢复，只是模型看不见它。
		paused, ok := r.Get("paused")
		if !ok || paused.Enabled {
			t.Fatalf("停用技能应存在且标记未启用: %+v", paused)
		}
		if body, err := r.Content("paused"); err != nil || !strings.Contains(body, "正文") {
			t.Fatalf("停用技能的正文取不到: err=%v body=%q", err, body)
		}
	})

	// 切换工作目录必须换掉工作区技能：留着上一个目录那套，用户会看到一堆打不开的技能。
	t.Run("来源优先级与工作区切换", func(t *testing.T) {
		global := t.TempDir()
		writeSkill(t, global, "plain", "---\nname: plain\ndescription: 全局技能。\n---\n\n正文\n")

		r := New()
		for _, s := range LoadDir(global, domain.SkillSourceGlobal) {
			r.Add(s)
		}

		old, neu := t.TempDir(), t.TempDir()
		first := writeSkill(t, filepath.Join(old, ".pi", "skills"), "ws-skill", "---\nname: ws-skill\ndescription: 工作区技能。\n---\n\n正文\n")
		writeSkill(t, filepath.Join(neu, ".pi", "skills"), "ws-skill", "---\nname: ws-skill\ndescription: 工作区技能。\n---\n\n正文\n")

		LoadWorkspace(r, old)
		sk, ok := r.Get("ws-skill")
		if !ok || sk.Source != domain.SkillSourceWorkspace {
			t.Fatalf("工作区技能没有装入: %+v", sk)
		}
		// 工作区技能读的是 PI 自己扫的目录：两边必须是同一个地方。
		if want := filepath.Join(old, ".pi", "skills", "ws-skill", "SKILL.md"); sk.Location != want {
			t.Fatalf("工作区技能目录应为 .pi/skills，实际 %s", sk.Location)
		}
		if sk.Location != first {
			t.Fatalf("装入的位置与写入的位置不一致: %s vs %s", sk.Location, first)
		}

		LoadWorkspace(r, neu)
		sk, ok = r.Get("ws-skill")
		if !ok || sk.Location == first {
			t.Fatalf("重载后仍指向上一个工作区: %+v", sk)
		}
		// 重载只换工作区那部分：全局技能一条都不能少。
		if n := len(r.List()); n != 2 {
			t.Fatalf("重载后应是 1 个全局技能 + 1 个工作区技能，实际 %d", n)
		}

		LoadWorkspace(r, "")
		if _, ok := r.Get("ws-skill"); ok {
			t.Fatal("工作目录为空时应摘掉工作区技能")
		}
		if _, ok := r.Get("plain"); !ok {
			t.Fatal("全局技能不该被工作区重载清掉")
		}
	})
}
