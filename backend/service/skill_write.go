// Skill 的写入侧：新建、导入目录与 zip、删除，落盘后立刻重扫注册表。
package service

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/skill"
)

// Create 新建一个用户技能，写进 PI 的 skills 目录并立刻注册。
func (s *SkillService) Create(req domain.CreateSkillREQ) (*domain.SkillVO, error) {
	if s.env.Skills == nil {
		return nil, domain.ErrSkillNotFound
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, pkg.New("8105", "给这个技能起个名字", "")
	}
	// 名字先过词法校验再拼路径：不带这一步，`..` 与路径分隔符能写出技能目录之外。
	if !skill.ValidName(name) {
		return nil, pkg.New("8101", "技能名只能用小写字母、数字和连字符", name)
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, pkg.New("8105", "技能内容不能为空", "")
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = firstLineOf(body)
	}

	dir := filepath.Join(s.env.PiEnv.SkillsDir(), name)
	if err := pkg.EnsureDir(dir); err != nil {
		return nil, err
	}
	raw := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, desc, body)
	path := filepath.Join(dir, "SKILL.md")
	if err := pkg.WriteText(path, raw); err != nil {
		return nil, err
	}

	parsed, err := skill.Parse(path, domain.SkillSourceGlobal)
	if err != nil {
		return nil, err
	}
	s.env.Skills.Add(parsed)
	s.Reload(s.env.Workspace())
	vo := parsed.ToVO()
	return &vo, nil
}

// Import 从磁盘导入技能：支持 SKILL.md 文件、含 SKILL.md 的文件夹、
// 以及装满技能的大目录（只取一层）。文件夹里的脚本与素材一起复制。
func (s *SkillService) Import(paths []string) (*domain.ImportSkillsRESP, error) {
	if s.env.Skills == nil {
		return nil, domain.ErrSkillNotFound
	}
	resp := &domain.ImportSkillsRESP{Skipped: []string{}}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		found := locateSkillFiles(p)
		if len(found) == 0 {
			resp.Skipped = append(resp.Skipped, filepath.Base(p))
			continue
		}
		for _, src := range found {
			if _, err := s.copySkillDir(src); err != nil {
				resp.Skipped = append(resp.Skipped, filepath.Base(filepath.Dir(src))+"："+err.Error())
				pkg.Warnf("skill: 导入 %s 失败: %v", src, err)
				continue
			}
			resp.Imported++
		}
	}
	s.Reload(s.env.Workspace())
	return resp, nil
}

// ImportZip 导入一个技能压缩包：zip 里可以是一个技能（任意层级的 SKILL.md），
// 也可以是一包技能。先解压到临时目录，再走与磁盘导入同一套定位与复制。
func (s *SkillService) ImportZip(filename string, data []byte) (*domain.ImportSkillsRESP, error) {
	if s.env.Skills == nil {
		return nil, domain.ErrSkillNotFound
	}
	if !strings.EqualFold(filepath.Ext(filename), ".zip") {
		return nil, pkg.New("8108", "技能包必须是 .zip 文件", filename)
	}
	tmp, err := os.MkdirTemp("", "workbaby-skill-")
	if err != nil {
		return nil, pkg.Wrap("8108", "创建临时目录失败", err)
	}
	defer os.RemoveAll(tmp)
	if err := unzipSkill(data, tmp); err != nil {
		return nil, err
	}
	found := locateSkillFiles(tmp)
	if len(found) == 0 {
		return nil, pkg.New("8109", "压缩包里没有 SKILL.md：每个技能一个文件夹，各含一个 SKILL.md", filename)
	}
	resp := &domain.ImportSkillsRESP{Skipped: []string{}}
	for _, src := range found {
		if _, err := s.copySkillDir(src); err != nil {
			resp.Skipped = append(resp.Skipped, filepath.Base(filepath.Dir(src))+"："+err.Error())
			continue
		}
		resp.Imported++
	}
	s.Reload(s.env.Workspace())
	return resp, nil
}

// 解压上限：技能包是文档不是数据载体，超限几乎肯定是拿错了文件。
const (
	zipMaxFileSize = 64 << 20
	zipMaxTotal    = 256 << 20
)

// unzipSkill 解压技能包到 dst。条目路径必须仍落在 dst 内，防 zip 路径穿越。
func unzipSkill(data []byte, dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return pkg.Wrap("8108", "压缩包打不开，确认是有效的 zip 文件", err)
	}
	var total int64
	for _, f := range zr.File {
		target, err := pkg.SafeJoin(dst, f.Name)
		if err != nil {
			return pkg.New("8108", "压缩包里有不合法的路径条目", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := pkg.EnsureDir(target); err != nil {
				return err
			}
			continue
		}
		// 符号链接条目必须拦掉：写文件时它会被当成普通文件写出内容。
		if f.Mode()&os.ModeSymlink != 0 {
			return pkg.New("8108", "压缩包里有符号链接条目，不安全的技能包", f.Name)
		}
		if err := pkg.EnsureDir(filepath.Dir(target)); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return pkg.Wrap("8108", "读取压缩包条目失败", err)
		}
		buf, rerr := io.ReadAll(io.LimitReader(rc, zipMaxFileSize+1))
		_ = rc.Close()
		if rerr != nil {
			return pkg.Wrap("8108", "读取压缩包条目失败", rerr)
		}
		if int64(len(buf)) > zipMaxFileSize {
			return pkg.New("8108", "压缩包里的单个文件太大", f.Name)
		}
		total += int64(len(buf))
		if total > zipMaxTotal {
			return pkg.New("8108", "压缩包解压后的总量太大", "")
		}
		if err := os.WriteFile(target, buf, 0o644); err != nil {
			return pkg.Wrap("8108", "解压压缩包失败", err)
		}
	}
	return nil
}

// copySkillDir 把一个技能目录整体复制进 PI 的 skills 目录，名字冲突时自动加序号。
func (s *SkillService) copySkillDir(src string) (string, error) {
	parsed, err := skill.Parse(src, domain.SkillSourceGlobal)
	if err != nil {
		return "", err
	}
	name := parsed.Name
	root := s.env.PiEnv.SkillsDir()
	dir := filepath.Join(root, name)
	for i := 2; pkg.DirExists(dir); i++ {
		name = fmt.Sprintf("%s-%d", parsed.Name, i)
		dir = filepath.Join(root, name)
	}
	// SKILL.md 所在目录就是技能根：脚本与 references 一起搬过去。
	srcDir := filepath.Dir(src)
	if err := copyTree(srcDir, dir); err != nil {
		return "", err
	}
	return name, nil
}

// Delete 删除用户技能；工作区与包里的技能不从这里删。
func (s *SkillService) Delete(id string) error {
	if s.env.Skills == nil {
		return domain.ErrSkillNotFound
	}
	sk, ok := s.env.Skills.Get(id)
	if !ok {
		return domain.ErrSkillNotFound
	}
	switch sk.Source {
	case domain.SkillSourceWorkspace:
		return pkg.New("8106", "工作区技能请到项目的 .pi/skills 里删除", "")
	case domain.SkillSourcePackage:
		return pkg.New("8112", "这个技能随插件包安装，请卸掉那个包", "")
	}
	// 用 Location 反推目录：导入时同名会自动加序号，重算 skillsDir/name 会指错地方。
	if dir := filepath.Dir(sk.Location); dir != "" && dir != "." {
		if err := os.RemoveAll(dir); err != nil {
			return pkg.Wrap("8107", "删除技能失败", err)
		}
	}
	s.env.Skills.Remove(id)
	s.Reload(s.env.Workspace())
	return nil
}

// locateSkillFiles 定位一个路径下可导入的 SKILL.md（最多一层）。
func locateSkillFiles(path string) []string {
	if !pkg.FileExists(path) && !pkg.DirExists(path) {
		return nil
	}
	if pkg.FileExists(path) {
		if strings.EqualFold(filepath.Base(path), "SKILL.md") {
			return []string{path}
		}
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			sub := filepath.Join(path, e.Name(), "SKILL.md")
			if pkg.FileExists(sub) {
				out = append(out, sub)
			}
			continue
		}
		if strings.EqualFold(e.Name(), "SKILL.md") {
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	return out
}

func firstLineOf(s string) string {
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len([]rune(line)) > 60 {
		return string([]rune(line)[:60]) + "…"
	}
	return line
}
