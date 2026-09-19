package service

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/skill"
)

// 技能包 zip 导入：
// 约定：每个含 SKILL.md 的目录即一个 Skill；
// <pkg>/scripts/ 下的脚本（.js/.ts/.py/.ps1/.sh）随包入库为可执行 scripts（run_skill_script 工具使用）。
// 单个失败不阻断整体；与内置 Skill 同名的包跳过（内置只读，避免被覆盖后启动期反复回写）。

// skillZipMaxBytes zip 上限。
const skillZipMaxBytes = 40 << 20

// skillZipFail 单个包导入失败详情。
type skillZipFail struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// ImportZip 从本地 zip 批量导入技能包，返回 { imported, skipped, failed } 概览。
func (s *SkillService) ImportZip(ctx context.Context, zipPath string) (map[string]any, error) {
	if strings.TrimSpace(zipPath) == "" {
		return nil, pkg.New(8002, "zip path is required", "")
	}
	info, err := os.Stat(zipPath)
	if err != nil {
		return nil, pkg.Wrap(8002, "zip not found", err)
	}
	if info.IsDir() {
		return nil, pkg.New(8002, "expected a zip file, got a directory", "")
	}
	if info.Size() > skillZipMaxBytes {
		return nil, pkg.New(8002, "zip exceeds 40MB limit", "")
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, pkg.Wrap(8002, "open zip failed", err)
	}
	defer zr.Close()

	// 按目录分组（zip 根目录下的文件归到 "."）
	filesByDir := map[string][]*zip.File{}
	for _, f := range zr.File {
		n := cleanZipName(f.Name)
		if n == "" || f.FileInfo().IsDir() {
			continue
		}
		d := path.Dir(n)
		filesByDir[d] = append(filesByDir[d], f)
	}

	imported, skipped := []string{}, []string{}
	failed := []skillZipFail{}

	dirs := make([]string, 0, len(filesByDir))
	for d := range filesByDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	for _, d := range dirs {
		files := filesByDir[d]
		skillmd := findZipFile(files, d, "SKILL.md")
		if skillmd == nil {
			continue // 无 SKILL.md 的目录（资源/README 等）跳过
		}
		if err := s.importSkillPackage(ctx, zipPath, d, skillmd, files, &imported, &failed); err != nil {
			skipped = append(skipped, err.Error())
		}
		if len(imported) == 0 && len(failed) == 0 {
			return nil, pkg.New(8002, "zip contains no skill package (need a directory with SKILL.md)", "")
		}
	}
	if err := s.Reload(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"imported": imported, "skipped": skipped, "failed": failed}, nil
}

// importSkillPackage 导入单个包：解析 SKILL.md → 收集 scripts → 去重 upsert。
func (s *SkillService) importSkillPackage(ctx context.Context, zipPath, dir string, skillmd *zip.File, files []*zip.File, imported *[]string, failed *[]skillZipFail) error {
	data, err := readZipBytes(skillmd)
	if err != nil {
		*failed = append(*failed, skillZipFail{zipEntry(dir, skillmd.Name), "read SKILL.md failed: " + err.Error()})
		return nil
	}
	sk, err := skill.ParseSKILLMD(data, domain.SkillSourceKindDownload, "zip:"+zipPath+":"+dir)
	if err != nil {
		*failed = append(*failed, skillZipFail{zipEntry(dir, skillmd.Name), err.Error()})
		return nil
	}
	// 内置同名保护：避免覆盖内置包
	if exist, _ := s.repo.GetByName(ctx, sk.Name); exist != nil && exist.SourceKind == domain.SkillSourceKindBuiltin {
		*failed = append(*failed, skillZipFail{zipEntry(dir, skillmd.Name), "skill '" + sk.Name + "' is builtin read-only, skipped"})
		return nil
	}
	// scripts/ 子目录脚本入库
	scripts, err := skillScriptsFromZip(files, dir)
	if err != nil {
		*failed = append(*failed, skillZipFail{zipEntry(dir, skillmd.Name), "read scripts failed: " + err.Error()})
		return nil
	}
	if len(scripts) > 0 {
		js, err := marshalScripts(scripts)
		if err != nil {
			*failed = append(*failed, skillZipFail{zipEntry(dir, skillmd.Name), err.Error()})
			return nil
		}
		sk.ScriptsJSON = js
	}
	sk.ID = pkg.NewID(domain.IDSkill)
	// 覆盖导入不重置用户关闭状态
	if exist, _ := s.repo.GetByName(ctx, sk.Name); exist != nil {
		sk.Enabled = exist.Enabled
	}
	if err := s.repo.Upsert(ctx, sk); err != nil {
		*failed = append(*failed, skillZipFail{zipEntry(dir, skillmd.Name), err.Error()})
		return nil
	}
	*imported = append(*imported, sk.Name)
	return nil
}

// skillScriptsFromZip 收集 <dir>/scripts/* 下的可执行脚本（name=去扩展名文件名）。
func skillScriptsFromZip(files []*zip.File, dir string) ([]domain.SkillScript, error) {
	prefix := ""
	if dir == "." {
		prefix = "scripts/"
	} else {
		prefix = dir + "/scripts/"
	}
	var out []domain.SkillScript
	for _, f := range files {
		n := cleanZipName(f.Name)
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		base := path.Base(n)
		lang, ok := zipScriptLanguage(base)
		if !ok {
			continue
		}
		data, err := readZipBytes(f)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.SkillScript{Name: strings.TrimSuffix(base, path.Ext(base)), Language: lang, Code: string(data)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// zipScriptLanguage 脚本扩展名 → 语言（与 run_skill_script 解释器映射一致）。
// 与目录发现 skill.ScriptLanguageForFile 共用同一张表，避免两条导入路径对同一文件漂移。
func zipScriptLanguage(name string) (string, bool) { return skill.ScriptLanguageForFile(name) }

// findZipFile 在 dir（目录名，根目录为 "."）下找 basename 命中的文件。
func findZipFile(files []*zip.File, dir, base string) *zip.File {
	for _, f := range files {
		if path.Dir(cleanZipName(f.Name)) != dir {
			continue
		}
		if path.Base(cleanZipName(f.Name)) == base {
			return f
		}
	}
	return nil
}

// cleanZipName zip 内路径统一为正斜杠并去首斜杠。
func cleanZipName(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/")
}

// zipEntry 失败定位：包目录 + zip 内路径。
func zipEntry(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

func readZipBytes(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, skillZipMaxBytes))
}
