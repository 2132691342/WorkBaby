package tests

import (
	"os"
	"path/filepath"
	"testing"

	"WorkBaby/backend/pkg"
)

// TestBuiltinSkillFrontmatter 断言每个内置技能的描述都能解析出来。
// 描述是引擎决定「何时触发这个技能」的唯一依据：解析不出来等于这个技能没有触发条件。
// 块标量（`description: |` 与 `>`）是翻车重灾区——只切当前行的话会得到一个字符。
func TestBuiltinSkillFrontmatter(t *testing.T) {
	root := filepath.Join(repoAssetsDir(t), "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("读内置技能资产失败：%v", err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		count++
		file := filepath.Join(root, entry.Name(), "SKILL.md")
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("技能 %s 没有 SKILL.md：%v", entry.Name(), err)
			continue
		}
		fm := pkg.ParseFrontmatter(string(raw))
		if fm.Name == "" {
			t.Errorf("技能 %s 解析不出 name", entry.Name())
		}
		if len(fm.Description) < 10 {
			t.Errorf("技能 %s 的描述解析成了 %d 个字符（块标量没接上）：%q",
				entry.Name(), len(fm.Description), fm.Description)
		}
	}
	if count == 0 {
		t.Fatal("内置技能资产目录是空的")
	}
}
