// 技能注册表链路：工作区切换与停用区的可见性。
// 坏了的表现：换工作目录后技能不跟着换、旧工作区的技能残留、
// 停用的技能从列表里消失（用户就再也开不回来了）。
package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

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

// TestSkillRegistryChain 是 skill 包唯一的测试入口。
func TestSkillRegistryChain(t *testing.T) {
	// 重载只换工作区那部分：全局技能一条都不能少，工作区技能不能留上一家的。
	// 停用区的技能也要列得出、正文可读——界面要能一键恢复它。
	t.Run("工作区技能随目录切换、停用区可见可读、全局技能不被清掉", func(t *testing.T) {
		globalDir := t.TempDir()
		writeSkill(t, globalDir, "office-docs", "---\nname: office-docs\ndescription: 在本机处理 Word / Excel。\n---\n\n正文\n")
		disabled := filepath.Join(t.TempDir(), "skills.disabled")
		writeSkill(t, disabled, "paused", "---\nname: paused\ndescription: 停用中的技能。\n---\n\n正文\n")

		r := New()
		for _, s := range LoadDir(globalDir, domain.SkillSourceGlobal) {
			r.Add(s)
		}
		for _, s := range LoadDisabledDir(disabled, domain.SkillSourceGlobal) {
			r.Add(s)
		}
		if n := len(r.List()); n != 2 {
			t.Fatalf("应装入 2 个技能（1 启用 + 1 停用），实际 %d", n)
		}
		paused, ok := r.Get("paused")
		if !ok || paused.Enabled {
			t.Fatalf("停用技能应存在且标记未启用: %+v", paused)
		}
		if body, err := r.Content("paused"); err != nil || !strings.Contains(body, "正文") {
			t.Fatalf("停用技能的正文取不到: err=%v body=%q", err, body)
		}

		// 工作区技能读的是 PI 自己扫的目录（.pi/skills）；换目录必须换掉它。
		old, neu := t.TempDir(), t.TempDir()
		first := writeSkill(t, filepath.Join(old, ".pi", "skills"), "ws-skill",
			"---\nname: ws-skill\ndescription: 工作区技能。\n---\n\n正文\n")
		writeSkill(t, filepath.Join(neu, ".pi", "skills"), "ws-skill",
			"---\nname: ws-skill\ndescription: 工作区技能。\n---\n\n正文\n")

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

	// frontmatter 解析失败会被静默跳过：用户看不到任何提示，技能等于没装。
	// 标量、列表、行内数组三种写法在真实技能里都常见，收进 map[string]string 会整份解析失败。
	t.Run("frontmatter 混合类型照样解析，坏文件只跳自己不中断扫描", func(t *testing.T) {
		dir := t.TempDir()
		writeSkill(t, dir, "office-docs", `---
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
`)
		// 一个坏文件：不能让它把整次扫描带崩。
		if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("没有 frontmatter"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := LoadDir(dir, domain.SkillSourceGlobal)
		if len(got) != 1 {
			t.Fatalf("应只装入 1 个可用技能，实际 %d: %+v", len(got), got)
		}
		if s := got[0]; s.Description == "" || len(s.WhenToUse) != 2 {
			t.Fatalf("frontmatter 没解析全: %+v", s)
		}
	})
}
