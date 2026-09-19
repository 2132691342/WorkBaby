package skill

import (
	"io/fs"
	"sort"

	"WorkBaby/assets"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// LoadBuiltin 从 embed FS 读取内置 Skill（assets/skills/{name}/SKILL.md）。
// 单个解析失败降级跳过（记日志由调用方决定）；返回可 upsert 进 skills 表的列表。
func LoadBuiltin() ([]domain.SkillDO, error) {
	entries, err := fs.ReadDir(assets.Skills, "skills")
	if err != nil {
		return nil, pkg.Wrap(8002, "read builtin skills dir failed", err)
	}
	var out []domain.SkillDO
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		data, err := assets.Skills.ReadFile("skills/" + name + "/SKILL.md")
		if err != nil {
			pkg.L.Warn("builtin skill missing SKILL.md", "name", name, "err", err.Error())
			continue
		}
		sk, err := ParseSKILLMD(data, domain.SkillSourceKindBuiltin, "assets/skills/"+name)
		if err != nil {
			pkg.L.Warn("parse builtin skill failed", "name", name, "err", err.Error())
			continue
		}
		sk.ID = pkg.NewID(domain.IDSkill)
		out = append(out, *sk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
