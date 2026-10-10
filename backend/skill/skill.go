// Package skill 解析 SKILL.md 并维护注册中心。文件即真相源，不落库。
package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"gopkg.in/yaml.v3"
)

// nameRe 限定技能名，保证 /skill:名字 的词法边界清晰。
var nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidName 报告一个名字能否用作技能目录名与 /skill: 词法名：
// 只允许小写字母、数字与连字符，最长 64 字符——路径分隔符与「..」天然被拒。
func ValidName(name string) bool {
	return name != "" && len(name) <= 64 && nameRe.MatchString(name)
}

// Registry 是技能注册中心；写只在加载期与设置页发生，读是高频路径。
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*domain.Skill
	order  []string
	// bodies 存内置技能的正文。embed 出来的 Location 是虚拟路径，
	// 模型的 read 工具读不到，必须把正文留在内存里。
	bodies map[string]string
}

// New 构造空注册中心。
func New() *Registry {
	return &Registry{skills: map[string]*domain.Skill{}, bodies: map[string]string{}}
}

// Add 加入一个技能；同名按来源优先级覆盖（workspace > global > builtin）。
func (r *Registry) Add(s domain.Skill) { r.AddWithBody(s, "") }

// AddWithBody 加入技能并预置正文；body 为空表示正文在磁盘上，按需再读。
func (r *Registry) AddWithBody(s domain.Skill, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := s.Name + "@" + s.Source
	if body != "" {
		r.bodies[key] = body
	} else {
		delete(r.bodies, key)
	}
	if _, ok := r.skills[key]; !ok {
		r.order = append(r.order, key)
	}
	r.skills[key] = &s
}

// List 按来源与名称排序列出技能。
func (r *Registry) List() []domain.Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Skill, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, *r.skills[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return sourceRank(out[i].Source) < sourceRank(out[j].Source)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Get 按 id（name@source）或名字取技能；同名多来源时返回优先级最高的那个。
// 两种形态都要认：前端拿到的就是 id，只认名字会让「删除」这类按 id 调用的操作永远落空。
func (r *Registry) Get(idOrName string) (*domain.Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *domain.Skill
	for _, k := range r.order {
		s := r.skills[k]
		if k != idOrName && s.Name != idOrName {
			continue
		}
		if best == nil || sourceRank(s.Source) > sourceRank(best.Source) {
			best = s
		}
	}
	if best == nil {
		return nil, false
	}
	cp := *best
	return &cp, true
}

// SetEnabled 启停技能；按 id（name@source）或名字匹配。
func (r *Registry) SetEnabled(idOrName string, enabled bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ok := false
	for _, k := range r.order {
		if k == idOrName || r.skills[k].Name == idOrName {
			r.skills[k].Enabled = enabled
			ok = true
		}
	}
	return ok
}

// DisabledNames 列出当前被停用的技能名，供服务层落库。
func (r *Registry) DisabledNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, 4)
	for _, k := range r.order {
		if !r.skills[k].Enabled {
			out = append(out, r.skills[k].Name)
		}
	}
	return out
}

// Remove 从注册表里移除一个技能。磁盘文件由调用方负责。
func (r *Registry) Remove(idOrName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.order[:0]
	removed := false
	for _, k := range r.order {
		if k == idOrName || r.skills[k].Name == idOrName {
			delete(r.skills, k)
			delete(r.bodies, k)
			removed = true
			continue
		}
		kept = append(kept, k)
	}
	r.order = kept
	return removed
}

// DropSource 摘掉某个来源的全部技能；切换工作目录时用，避免新旧目录的技能混在一起。
func (r *Registry) DropSource(source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.order[:0]
	for _, k := range r.order {
		if r.skills[k].Source == source {
			delete(r.skills, k)
			delete(r.bodies, k)
			continue
		}
		kept = append(kept, k)
	}
	r.order = kept
}

// LoadWorkspace 换入一个工作区的技能：先摘掉上一个工作区的，再装入新的。
// workspace 为空表示不再使用工作区技能，必须摘干净。
func LoadWorkspace(r *Registry, workspace string) {
	if r == nil {
		return
	}
	r.DropSource(domain.SkillSourceWorkspace)
	if workspace == "" {
		return
	}
	dir := filepath.Join(workspace, ".workbaby", "skills")
	for _, s := range LoadDir(dir, domain.SkillSourceWorkspace) {
		r.Add(s)
	}
}

// ApplyDisabled 按停用名单设置启停状态，启动期调用。
func (r *Registry) ApplyDisabled(names []string) {
	if len(names) == 0 {
		return
	}
	blocked := make(map[string]bool, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			blocked[n] = true
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, k := range r.order {
		if blocked[r.skills[k].Name] {
			r.skills[k].Enabled = false
		}
	}
}

// Render 渲染进系统提示的技能清单：只给 name / description / location，正文由模型按需 read。
func (r *Registry) Render() string {
	list := r.List()
	enabled := make([]domain.Skill, 0, len(list))
	for _, s := range list {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	if len(enabled) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range enabled {
		b.WriteString("  <skill name=\"" + s.Name + "\" location=\"" + s.Location + "\">\n")
		b.WriteString("    " + s.Description + "\n")
		// 触发词真的给到模型才算数：写入 frontmatter 却没人读，等于没写。
		if len(s.WhenToUse) > 0 {
			b.WriteString("    适用于：" + strings.Join(s.WhenToUse, " / ") + "\n")
		}
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>\n")
	b.WriteString("需要用到某个技能时，用 read 打开它的 SKILL.md 再照着做。\n")
	return b.String()
}

// Content 读技能正文：内置技能从内存取，磁盘技能按路径读。
func (r *Registry) Content(name string) (string, error) {
	s, ok := r.Get(name)
	if !ok {
		return "", domain.ErrSkillNotFound
	}
	r.mu.RLock()
	body := r.bodies[s.Name+"@"+s.Source]
	r.mu.RUnlock()
	if body != "" {
		return body, nil
	}
	raw, err := os.ReadFile(s.Location)
	if err != nil {
		return "", pkg.Wrap(8104, "读取技能文件失败", err)
	}
	return stripFrontmatter(string(raw)), nil
}

// LoadDir 扫描一个目录下的 SKILL.md；遇到 SKILL.md 的目录不再向下递归。
func LoadDir(dir, source string) []domain.Skill {
	out := []domain.Skill{}
	if dir == "" || !pkg.DirExists(dir) {
		return out
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		s, perr := Parse(path, source)
		if perr != nil {
			pkg.Warnf("skill: 跳过无法解析的技能 %s: %v", path, perr)
			return nil
		}
		out = append(out, s)
		return filepath.SkipDir
	})
	return out
}

// Embedded 是内置技能：Raw 是 SKILL.md 原文，Body 是去掉开头说明的方法论正文。
type Embedded struct {
	Skill domain.Skill
	Body  string
	Raw   string
}

// LoadFS 从嵌入文件系统加载内置技能。
// root 是 embed.FS 里的相对目录（不是仓库路径），传错只会静默返回空列表。
func LoadFS(fsys fs.FS, root, source string) []Embedded {
	out := []Embedded{}
	_ = fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		raw, rerr := fs.ReadFile(fsys, path)
		if rerr != nil {
			return nil
		}
		s, perr := ParseContent(string(raw), path, source)
		if perr != nil {
			pkg.Warnf("skill: 跳过无法解析的内置技能 %s: %v", path, perr)
			return nil
		}
		out = append(out, Embedded{Skill: s, Body: stripFrontmatter(string(raw)), Raw: string(raw)})
		return nil
	})
	return out
}

// InstallBuiltin 把内置技能写到磁盘再登记：embed 的虚拟路径模型读不到，必须是真文件。
// 写失败则退化为「正文留在内存」，显式调用仍可用。
func InstallBuiltin(r *Registry, fsys fs.FS, root, dir string) {
	for _, e := range LoadFS(fsys, root, domain.SkillSourceBuiltin) {
		target := filepath.Join(dir, e.Skill.Name, "SKILL.md")
		if err := pkg.WriteText(target, e.Raw); err != nil {
			pkg.Warnf("skill: 内置技能 %s 落盘失败，正文只留在内存: %v", e.Skill.Name, err)
			r.AddWithBody(e.Skill, e.Body)
			continue
		}
		e.Skill.Location = target
		r.AddWithBody(e.Skill, e.Body)
	}
}

// Parse 解析磁盘上的一个 SKILL.md：YAML frontmatter 给元信息，正文是方法论。
func Parse(path, source string) (domain.Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Skill{}, pkg.Wrap(8102, "读取技能文件失败", err)
	}
	return ParseContent(string(raw), path, source)
}

// ParseContent 解析技能正文：YAML frontmatter 给元信息，其余是方法论。
func ParseContent(text, location, source string) (domain.Skill, error) {
	// frontmatter 里可能混有列表、布尔、数字（when_to_use、version…），
	// 收进 map[string]string 会让整份 YAML 解析失败、技能被静默跳过。
	meta := map[string]any{}
	body := text

	if strings.HasPrefix(strings.TrimSpace(text), "---") {
		rest := strings.TrimSpace(text)
		rest = strings.TrimPrefix(rest, "---")
		if idx := strings.Index(rest, "\n---"); idx >= 0 {
			head := rest[:idx]
			body = strings.TrimSpace(rest[idx+4:])
			if err := yaml.Unmarshal([]byte(head), &meta); err != nil {
				return domain.Skill{}, pkg.Wrap(8102, "技能开头说明格式不正确", err)
			}
		}
	}

	name := strings.TrimSpace(scalar(meta["name"]))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filepath.Dir(location)), ".md")
	}
	if !ValidName(name) {
		return domain.Skill{}, pkg.Wrap(8101, "技能名不合法", domain.ErrSkillName)
	}
	desc := strings.TrimSpace(scalar(meta["description"]))
	if desc == "" {
		desc = firstLine(body)
	}
	return domain.Skill{
		ID:          name + "@" + source,
		Name:        name,
		Description: desc,
		WhenToUse:   stringsOf(meta["when_to_use"]),
		Location:    location,
		Source:      source,
		Enabled:     true,
	}, nil
}

// stripFrontmatter 去掉正文开头的 frontmatter，只留方法论部分。
func stripFrontmatter(text string) string {
	if !strings.HasPrefix(strings.TrimSpace(text), "---") {
		return text
	}
	rest := strings.TrimSpace(text)
	rest = strings.TrimPrefix(rest, "---")
	if idx := strings.Index(rest, "\n---"); idx >= 0 {
		return strings.TrimSpace(rest[idx+4:])
	}
	return text
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return strings.TrimSpace(s)
}

// stringsOf 把 frontmatter 里可能是标量或列表的值收成字符串列表（when_to_use 两种写法都合法）。
func stringsOf(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		if s := strings.TrimSpace(t); s != "" {
			return []string{s}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, it := range t {
			if s := strings.TrimSpace(scalar(it)); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func scalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func sourceRank(s string) int {
	switch s {
	case domain.SkillSourceWorkspace:
		return 3
	case domain.SkillSourceGlobal:
		return 2
	default:
		return 1
	}
}
