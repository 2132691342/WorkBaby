package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// DiscoverDir 扫描目录约定的技能：<root>/<name>/SKILL.md（+ 可选 <name>/scripts/*）。
//
// 单个技能解析失败降级跳过，不阻断其余技能装载；根目录不存在返回空列表
// （未创建过技能目录是首次启动的正常状态）。
func DiscoverDir(root string, kind domain.SkillSourceKind) []domain.SkillDO {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			pkg.L.Warn("read skill dir failed", "root", root, "err", err.Error())
		}
		return nil
	}
	out := make([]domain.SkillDO, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		data, rerr := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if rerr != nil {
			pkg.L.Warn("skill dir missing SKILL.md", "dir", dir, "err", rerr.Error())
			continue
		}
		sk, perr := ParseSKILLMD(data, kind, dir)
		if perr != nil {
			pkg.L.Warn("parse skill dir failed", "dir", dir, "err", perr.Error())
			continue
		}
		sk.ID = pkg.NewID(domain.IDSkill)
		if scripts := discoverScripts(dir); len(scripts) > 0 {
			if bs, merr := marshalScriptsJSON(scripts); merr == nil {
				sk.ScriptsJSON = bs
			}
		}
		out = append(out, *sk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// discoverScripts 收集 <dir>/scripts/* 下的脚本；name = 去扩展名文件名，
// 与 run_skill_script 的 script 参数一致。
func discoverScripts(dir string) []domain.SkillScript {
	files, err := os.ReadDir(filepath.Join(dir, "scripts"))
	if err != nil {
		return nil
	}
	out := make([]domain.SkillScript, 0, len(files))
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		base := f.Name()
		lang, ok := ScriptLanguageForFile(base)
		if !ok {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, "scripts", base))
		if rerr != nil {
			pkg.L.Warn("read skill script failed", "dir", dir, "script", base, "err", rerr.Error())
			continue
		}
		out = append(out, domain.SkillScript{
			Name:     strings.TrimSuffix(base, filepath.Ext(base)),
			Language: lang,
			Code:     string(data),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ScriptLanguageForFile 脚本扩展名 → 语言；与 run_skill_script 解释器映射一致，
// zip 导入与目录发现共用同一张表。
func ScriptLanguageForFile(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js", ".ts", ".mjs":
		return "javascript", true
	case ".py":
		return "python", true
	case ".ps1":
		return "powershell", true
	case ".sh":
		return "bash", true
	default:
		return "", false
	}
}

// marshalScriptsJSON 序列化脚本列表；空列表返回空串（= 无脚本）。
func marshalScriptsJSON(scripts []domain.SkillScript) (string, error) {
	if len(scripts) == 0 {
		return "", nil
	}
	bs, err := json.Marshal(scripts)
	if err != nil {
		return "", pkg.Wrap(8002, "marshal skill scripts failed", err)
	}
	return string(bs), nil
}

// StaleSkillNames 找出「来源是扫描目录、但磁盘 SKILL.md 已消失」的技能名；
// 内置技能与 UI 自建技能（SourceRef 为空）不受影响。
func StaleSkillNames(rows []domain.SkillDO, roots []string) []string {
	var out []string
	for i := range rows {
		ref := strings.TrimSpace(rows[i].SourceRef)
		if ref == "" || !UnderAnyRoot(ref, roots) {
			continue
		}
		if _, err := os.Stat(filepath.Join(ref, "SKILL.md")); err != nil {
			out = append(out, rows[i].Name)
		}
	}
	return out
}

// UnderRoot ref 是否等于 root 或位于其下（Windows 大小写不敏感）。
func UnderRoot(ref, root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	r := strings.ToLower(filepath.Clean(ref))
	base := strings.ToLower(filepath.Clean(root))
	return r == base || strings.HasPrefix(r, base+string(filepath.Separator))
}

// UnderAnyRoot ref 是否位于任一 root 之下。
func UnderAnyRoot(ref string, roots []string) bool {
	for _, root := range roots {
		if UnderRoot(ref, root) {
			return true
		}
	}
	return false
}
