// 技能压缩包导入链路：解压落盘注册、zip 路径穿越护栏、空包拒绝。
package service

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/skill"
)

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newSkillEnv(t *testing.T) (*Env, *SkillService) {
	t.Helper()
	env, svc := newEnv(t)
	env.Skills = skill.New()
	return env, svc.Skills
}

func TestSkillZipImport(t *testing.T) {
	t.Run("多技能包解压落盘并注册", func(t *testing.T) {
		env, svc := newSkillEnv(t)
		data := buildZip(t, map[string]string{
			"a/SKILL.md": "---\nname: skill-a\ndescription: 技能 A\n---\n正文 A\n",
			"b/SKILL.md": "---\nname: skill-b\ndescription: 技能 B\n---\n正文 B\n",
		})
		resp, err := svc.ImportZip("pack.zip", data)
		if err != nil {
			t.Fatalf("导入失败: %v", err)
		}
		if resp.Imported != 2 {
			t.Fatalf("应导入 2 个技能，实际 %d", resp.Imported)
		}
		for _, name := range []string{"skill-a", "skill-b"} {
			if _, ok := env.Skills.Get(name); !ok {
				t.Fatalf("%s 未注册", name)
			}
			if !pkg.FileExists(filepath.Join(env.Paths.SkillsDir, name, "SKILL.md")) {
				t.Fatalf("%s 未落盘", name)
			}
		}
	})

	t.Run("条目穿越被拒绝且不落盘", func(t *testing.T) {
		env, svc := newSkillEnv(t)
		data := buildZip(t, map[string]string{
			"../evil.txt": "x",
			"a/SKILL.md":  "---\nname: skill-c\ndescription: c\n---\n正文\n",
		})
		if _, err := svc.ImportZip("evil.zip", data); pkg.CodeOf(err) != 8108 {
			t.Fatalf("应报 8108，实际 %v", err)
		}
		if pkg.FileExists(filepath.Join(filepath.Dir(env.Paths.SkillsDir), "evil.txt")) {
			t.Fatal("穿越文件竟然写出去了")
		}
	})

	t.Run("没有 SKILL.md 的包被拒绝", func(t *testing.T) {
		_, svc := newSkillEnv(t)
		data := buildZip(t, map[string]string{"readme.txt": "hi"})
		if _, err := svc.ImportZip("empty.zip", data); pkg.CodeOf(err) != 8109 {
			t.Fatalf("应报 8109，实际 %v", err)
		}
	})
}
