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
	// frontmatter 混合类型必须照样解析（解析失败会被静默跳过，用户看不到任何提示）；
	// 来源优先级与工作区切换在同一份注册表上验：重载只换工作区那部分，全局技能不动。
	t.Run("两区扫描、frontmatter 混合类型与工作区切换", func(t *testing.T) {
		dir := t.TempDir()
		writeSkill(t, dir, "office-docs", multiTypeFrontmatter)
		disabled := filepath.Join(t.TempDir(), "skills.disabled")
		writeSkill(t, disabled, "paused", "---\nname: paused\ndescription: 停用中的技能。\n---\n\n正文\n")

		r := New()
		for _, s := range LoadDir(dir, domain.SkillSourceGlobal) {
			r.Add(s)
		}
		for _, s := range LoadDisabledDir(disabled, domain.SkillSourceGlobal) {
			r.Add(s)
		}
		if n := len(r.List()); n != 2 {
			t.Fatalf("应装入 2 个技能（1 启用 + 1 停用），实际 %d", n)
		}
		off, ok := r.Get("office-docs")
		if !ok || off.Description == "" || !off.Enabled || len(off.WhenToUse) != 2 {
			t.Fatalf("启用技能没装进来或触发词没解析: %+v", off)
		}
		// 停用的技能也在列表里、正文可读——界面要列得出、能一键恢复，只是模型看不见它。
		paused, ok := r.Get("paused")
		if !ok || paused.Enabled {
			t.Fatalf("停用技能应存在且标记未启用: %+v", paused)
		}
		if body, err := r.Content("paused"); err != nil || !strings.Contains(body, "正文") {
			t.Fatalf("停用技能的正文取不到: err=%v body=%q", err, body)
		}

		// 工作区技能读的是 PI 自己扫的目录（.pi/skills）；换目录必须换掉它。
		old, neu := t.TempDir(), t.TempDir()
		first := writeSkill(t, filepath.Join(old, ".pi", "skills"), "ws-skill", "---\nname: ws-skill\ndescription: 工作区技能。\n---\n\n正文\n")
		writeSkill(t, filepath.Join(neu, ".pi", "skills"), "ws-skill", "---\nname: ws-skill\ndescription: 工作区技能。\n---\n\n正文\n")

		LoadWorkspace(r, old)
		sk, ok := r.Get("ws-skill")
		if !ok || sk.Source != domain.SkillSourceWorkspace || sk.Location != first {
			t.Fatalf("工作区技能没有装入或目录不对: %+v", sk)
		}

		LoadWorkspace(r, neu)
		sk, ok = r.Get("ws-skill")
		if !ok || sk.Location == first {
			t.Fatalf("重载后仍指向上一个工作区: %+v", sk)
		}
		// 重载只换工作区那部分：全局技能一条都不能少。
		if n := len(r.List()); n != 3 {
			t.Fatalf("重载后应是 2 个全局技能 + 1 个工作区技能，实际 %d", n)
		}

		LoadWorkspace(r, "")
		if _, ok := r.Get("ws-skill"); ok {
			t.Fatal("工作目录为空时应摘掉工作区技能")
		}
		if _, ok := r.Get("office-docs"); !ok {
			t.Fatal("全局技能不该被工作区重载清掉")
		}
	})
}
