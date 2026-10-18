// 设置编排：WorkBaby 自己的界面偏好（KV）与 PI 侧配置的读写（settings.json）。
// 两者的边界很硬：默认模型、工具集、压缩参数属于 PI，不再存第二份。
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/runtime"
)

// SettingsService 负责键值设置、背景图与 PI 配置的读写。
type SettingsService struct {
	env   *Env
	paths runtime.Paths
}

// NewSettingsService 构造设置服务。
func NewSettingsService(env *Env, paths runtime.Paths) *SettingsService {
	return &SettingsService{env: env, paths: paths}
}

// All 取全部设置，缺失项用默认值补齐。
func (s *SettingsService) All() (map[string]string, error) {
	stored, err := s.env.Repo.AllSettings()
	if err != nil {
		return nil, err
	}
	for k, v := range domain.DefaultSettings() {
		if _, ok := stored[k]; !ok {
			stored[k] = v
		}
	}
	if stored[domain.SettingWorkspace] == "" && s.env.Cfg.Workspace != "" {
		stored[domain.SettingWorkspace] = s.env.Cfg.Workspace
	}
	return stored, nil
}

// Set 写入一个设置。
func (s *SettingsService) Set(key, value string) error {
	if key == "" {
		return pkg.New("2006", "设置项名不能为空", "")
	}
	switch key {
	case domain.SettingTheme:
		if value != "light" && value != "dark" {
			return pkg.New("2007", "主题只有浅色和暗色两种", value)
		}
	case domain.SettingWorkspace:
		if value != "" && !pkg.DirExists(value) {
			return pkg.New("1003", "这个目录不存在", value)
		}
		if err := s.env.Cfg.SetWorkspace(value); err != nil {
			return err
		}
		// 换工作目录要重扫工作区技能：旧目录的技能不该留在列表里。
		NewSkillService(s.env).Reload(value)
	}
	return s.env.Repo.SetSetting(key, value)
}

// SeedDefaults 首启写入默认值。
func (s *SettingsService) SeedDefaults() error {
	for k, v := range domain.DefaultSettings() {
		cur, err := s.env.Repo.GetSetting(k)
		if err != nil {
			return err
		}
		if cur == "" {
			if err := s.env.Repo.SetSetting(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// PISettings 返回 PI 侧配置快照与全部文件位置。
func (s *SettingsService) PISettings() (*domain.PISettingsVO, error) {
	file, err := s.env.PiEnv.ReadSettings()
	if err != nil {
		return nil, err
	}
	if file == nil {
		file = &pienv.SettingsFile{}
	}
	vo := &domain.PISettingsVO{
		DefaultProvider: file.DefaultProvider, DefaultModel: file.DefaultModel,
		DefaultThinkingLevel: file.DefaultThinkingLevel,
		DefaultTools:         file.DefaultTools,
		AgentDir:             s.env.PiEnv.AgentDir(),
		SettingsPath:         s.env.PiEnv.SettingsPath(),
		ModelsPath:           s.env.PiEnv.ModelsPath(),
		MCPPath:              s.env.PiEnv.MCPPath(),
		SessionsDir:          s.env.PiEnv.SessionsDir(),
		ExtensionsDir:        s.env.PiEnv.ExtensionsDir(),
		SkillsDir:            s.env.PiEnv.SkillsDir(),
	}
	if vo.DefaultTools == nil {
		vo.DefaultTools = []string{}
	}
	if file.Compaction != nil {
		vo.CompactionReserve = file.Compaction.ReserveTokens
		vo.CompactionKeep = file.Compaction.KeepRecentTokens
	}
	return vo, nil
}

// SetThinkingLevel 写 PI 的默认思考档。
func (s *SettingsService) SetThinkingLevel(level string) error {
	switch level {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max", "":
	default:
		return pkg.New("1107", "思考档位不正确", level)
	}
	file, err := s.readSettings()
	if err != nil {
		return err
	}
	file.DefaultThinkingLevel = level
	return s.env.PiEnv.WriteSettings(file)
}

// SetCompaction 写 PI 的压缩参数；0 表示交回默认。
func (s *SettingsService) SetCompaction(req domain.SetCompactionREQ) error {
	if req.Reserve < 0 || req.Keep < 0 {
		return pkg.New("2008", "压缩参数不能是负数", "")
	}
	file, err := s.readSettings()
	if err != nil {
		return err
	}
	if file.Compaction == nil {
		file.Compaction = &pienv.CompactionSettings{}
	}
	file.Compaction.ReserveTokens = req.Reserve
	file.Compaction.KeepRecentTokens = req.Keep
	return s.env.PiEnv.WriteSettings(file)
}

// readSettings 读 PI 的 settings.json；不存在给空结构。
func (s *SettingsService) readSettings() (*pienv.SettingsFile, error) {
	file, err := s.env.PiEnv.ReadSettings()
	if err != nil {
		return nil, err
	}
	if file == nil {
		file = &pienv.SettingsFile{}
	}
	return file, nil
}

// Background 取当前自定义背景；没设置时 Image 为空串。
func (s *SettingsService) Background() (domain.BackgroundVO, error) {
	all, err := s.All()
	if err != nil {
		return domain.BackgroundVO{}, err
	}
	return domain.BackgroundVO{
		Image:   all[domain.SettingBgImage],
		Opacity: numOr(all[domain.SettingBgOpacity], 0.5),
		Blur:    numOr(all[domain.SettingBgBlur], 0),
	}, nil
}

// SetBackground 设置自定义背景：先把图片复制进数据目录再记路径与叠层参数，空路径表示清除。
// 引用原文件不行——原文件一删一挪背景就断。
func (s *SettingsService) SetBackground(path string, opacity, blur float64) (domain.BackgroundVO, error) {
	opacity = clampF(opacity, 0, domain.MaxBgOpacity)
	blur = clampF(blur, 0, domain.MaxBgBlurPx)
	if path == "" {
		if err := s.clearBackground(); err != nil {
			return domain.BackgroundVO{}, err
		}
		return domain.BackgroundVO{Opacity: opacity, Blur: blur}, nil
	}
	if !pkg.FileExists(path) {
		return domain.BackgroundVO{}, pkg.New("1005", "这张图片不存在，可能已经被挪走或删掉了", path)
	}
	if !bgExts[pkg.Ext(path)] {
		return domain.BackgroundVO{}, pkg.New("1014", "这张图不是能当背景的格式（支持 png / jpg / webp / bmp）", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return domain.BackgroundVO{}, pkg.Wrap("1005", "读取这张图片失败", err)
	}
	if st.Size() > domain.MaxBgBytes {
		return domain.BackgroundVO{}, pkg.New(
			"1013",
			fmt.Sprintf("这张图有 %d MB，超过 %d MB 的上限：换一张小一点的，或先压一下",
				st.Size()>>20, domain.MaxBgBytes>>20),
			path,
		)
	}
	dst := filepath.Join(s.paths.BackgroundDir, "background."+pkg.Ext(path))
	if err := pkg.CopyFile(path, dst); err != nil {
		return domain.BackgroundVO{}, err
	}
	s.removeBgFilesExcept(dst)

	// 三项要么都成要么都不成：只写了路径没写叠层参数，界面会按缺省值渲染出
	// 一张完全不透明的图，把正文盖住。
	if err := s.env.Repo.SetSettings(map[string]string{
		domain.SettingBgImage:   dst,
		domain.SettingBgOpacity: fmtF(opacity),
		domain.SettingBgBlur:    fmtF(blur),
	}); err != nil {
		return domain.BackgroundVO{}, err
	}
	return domain.BackgroundVO{Image: dst, Opacity: opacity, Blur: blur}, nil
}

// clearBackground 清掉背景设置并删除已存的副本。
func (s *SettingsService) clearBackground() error {
	if err := s.env.Repo.SetSettings(map[string]string{
		domain.SettingBgImage:   "",
		domain.SettingBgOpacity: "",
		domain.SettingBgBlur:    "",
	}); err != nil {
		return err
	}
	s.removeBgFilesExcept("")
	return nil
}

// removeBgFilesExcept 删掉背景目录里除 keep 之外的文件：换图与清除都会留下旧副本。
func (s *SettingsService) removeBgFilesExcept(keep string) {
	entries, err := os.ReadDir(s.paths.BackgroundDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(s.paths.BackgroundDir, e.Name())
		if full == keep {
			continue
		}
		if err := os.Remove(full); err != nil {
			pkg.Warnf("settings: 删除旧背景图失败 %s: %v", full, err)
		}
	}
}

// WorkspaceCandidates 返回最近用过的工作目录（从会话文件里取），供界面上快速切换。
func (s *SettingsService) WorkspaceCandidates() []string {
	out := []string{}
	seen := map[string]bool{}
	if cur := s.env.Workspace(); cur != "" {
		out = append(out, cur)
		seen[strings.ToLower(filepath.Clean(cur))] = true
	}
	metas, err := pi.ListSessions(s.env.PiEnv.SessionsDir())
	if err != nil {
		return out
	}
	for _, m := range metas {
		if m.Cwd == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(m.Cwd))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, m.Cwd)
	}
	return out
}

// bgExts 是允许作为背景的图片格式。
var bgExts = map[string]bool{"png": true, "jpg": true, "jpeg": true, "webp": true, "bmp": true}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func fmtF(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

func numOr(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}
