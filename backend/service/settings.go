package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/runtime"
)

// SettingsService 负责键值设置与外观项。
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
		return pkg.New(2006, "设置项名不能为空", "")
	}
	switch key {
	case domain.SettingTheme:
		if value != "light" && value != "dark" {
			return pkg.New(2007, "主题只有晴空和紫夜两种", value)
		}
	case domain.SettingPermission:
		switch value {
		case domain.PermissionAsk, domain.PermissionAutoEdit, domain.PermissionYolo:
		default:
			return pkg.New(1106, "权限档位不正确", value)
		}
	case domain.SettingWorkspace:
		if value != "" && !pkg.DirExists(value) {
			return pkg.New(1003, "这个目录不存在", value)
		}
		if err := s.env.Cfg.SetWorkspace(value); err != nil {
			return err
		}
		// 工作区技能跟着工作目录走：不重载的话切完目录技能列表还是旧的那套。
		s.env.ReloadWorkspaceSkills()
	}
	return s.env.Repo.SetSetting(key, value)
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

// SetBackground 设置自定义背景：先把图片复制进数据目录，再记路径与叠层参数；
// 空路径表示清除。引用用户选中的原文件是不行的——原文件一删一挪背景就没了，
// 而用户完全不记得自己当初选的是哪一个文件。
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
		return domain.BackgroundVO{}, pkg.Wrap(1005, "这张图片不存在，可能已经被挪走或删掉了", nil)
	}
	if !bgExts[pkg.Ext(path)] {
		return domain.BackgroundVO{}, pkg.New(1014, "这张图不是能当背景的格式（支持 png / jpg / webp / bmp）", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return domain.BackgroundVO{}, pkg.Wrap(1005, "读取这张图片失败", err)
	}
	if st.Size() > domain.MaxBgBytes {
		return domain.BackgroundVO{}, pkg.New(
			1013,
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

// removeBgFilesExcept 删掉背景目录里除 keep 之外的文件：换图与清除都会留下旧副本，
// 不清就是无上界的磁盘增长，而用户根本不知道数据目录里堆着一堆旧图。
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

// MigrateLegacy 清掉历史版本写死的默认值，让它们回到「跟随模型」的新语义。
// 存量库里这些值是首启时由旧默认写入的，不清理会永远盖住新逻辑。
func (s *SettingsService) MigrateLegacy() error {
	// 旧版把压缩余量写死 16384；现在缺省跟随模型真实输出预算。
	legacyReserve := "16384"
	if v, err := s.env.Repo.GetSetting(domain.SettingContextReserve); err == nil && v == legacyReserve {
		if err := s.env.Repo.SetSetting(domain.SettingContextReserve, ""); err != nil {
			return err
		}
	}
	return nil
}
