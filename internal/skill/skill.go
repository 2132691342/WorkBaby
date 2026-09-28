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

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"

	"gopkg.in/yaml.v3"
)

// nameRe 限定技能名，保证 /skill:名字 的词法边界清晰。
var nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

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
		return "", pkg.Wrap(6003, "读取技能文件失败", err)
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

// Embedded 是内置技能：正文随二进制嵌入，Location 是虚拟路径，Body 才拿得到正文。
type Embedded struct {
	Skill domain.Skill
	Body  string
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
		out = append(out, Embedded{Skill: s, Body: stripFrontmatter(string(raw))})
		return nil
	})
	return out
}

// Parse 解析磁盘上的一个 SKILL.md：YAML frontmatter 给元信息，正文是方法论。
func Parse(path, source string) (domain.Skill, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.Skill{}, pkg.Wrap(6102, "读取技能文件失败", err)
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
				return domain.Skill{}, pkg.Wrap(6102, "技能开头说明格式不正确", err)
			}
		}
	}

	name := strings.TrimSpace(scalar(meta["name"]))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filepath.Dir(location)), ".md")
	}
	if !nameRe.MatchString(name) || len(name) > 64 {
		return domain.Skill{}, pkg.Wrap(6101, "技能名不合法", domain.ErrSkillName)
	}
	desc := strings.TrimSpace(scalar(meta["description"]))
	if desc == "" {
		desc = firstLine(body)
	}
	return domain.Skill{
		ID:          name + "@" + source,
		Name:        name,
		Description: desc,
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

// scalar 把 YAML 标量取成字符串；我们只关心 name 与 description 这两个纯文本字段。
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
