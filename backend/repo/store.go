// Package repo 是持久化层：单个 JSON 文件承载壳的设置与技能开关台账。
// 它只依赖 pkg，不感知业务编排。
package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// Store 是壳自己的设置存储。会话、插件、技能都不在这里——
// 它们的真相在磁盘文件与引擎里，这里只存「壳的开关台账」。
// 条目量级是几十条：整读整写一个 JSON 文件加互斥锁足够，不值得引入数据库。
type Store struct {
	mu   sync.Mutex
	path string
	data fileData
}

// fileData 是 JSON 文件的整体形状。
type fileData struct {
	// Settings 设置 KV（值一律字符串，复杂结构自己 JSON）
	Settings map[string]string `json:"settings"`
	// SkillToggles 技能开关台账，键是技能目录名。开与关的真相是技能目录
	// 在哪个区（skills / skills_disabled），这份台账只做记账与核对。
	SkillToggles map[string]bool `json:"skill_toggles"`
}

// Open 打开（必要时创建）设置存储文件；内容整读进内存。
func Open(path string) (*Store, error) {
	if err := pkg.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, pkg.WrapErr(pkg.CodeStorage, "数据目录不可写", err)
	}
	s := &Store{path: path, data: fileData{
		Settings:     map[string]string{},
		SkillToggles: map[string]bool{},
	}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 首启就把文件立起来：数据目录不可写在启动当刻就暴露，
			// 而不是拖到第一次保存设置的时候。
			if werr := s.persist(); werr != nil {
				return nil, werr
			}
			return s, nil
		}
		return nil, pkg.WrapErr(pkg.CodeStorage, "设置文件读取失败", err)
	}
	// 文件坏了不堵启动：挪到旁边留证，从空白重新开始。
	// 半截 JSON 多半来自断电或杀进程，留着只会让每次启动都失败。
	if err := json.Unmarshal(raw, &s.data); err != nil {
		bad := fmt.Sprintf("%s.bad-%d", path, time.Now().UnixMilli())
		_ = os.Rename(path, bad)
		pkg.Warn("设置文件损坏已挪走：%s（%v）", bad, err)
	}
	if s.data.Settings == nil {
		s.data.Settings = map[string]string{}
	}
	if s.data.SkillToggles == nil {
		s.data.SkillToggles = map[string]bool{}
	}
	return s, nil
}

// persist 把内存态整写回文件。调用方必须已持有锁。
func (s *Store) persist() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "设置序列化失败", err)
	}
	if err := pkg.WriteFileAtomic(s.path, raw); err != nil {
		return pkg.WrapErr(pkg.CodeStorage, "设置保存失败", err)
	}
	return nil
}

// Get 读一个设置项；不存在返回空串与 false。
func (s *Store) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.Settings[key]
	return v, ok
}

// All 读出全部设置项的副本。
func (s *Store) All() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.data.Settings))
	for k, v := range s.data.Settings {
		out[k] = v
	}
	return out
}

// Save 批量写入设置项并落盘。
func (s *Store) Save(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range values {
		s.data.Settings[k] = v
	}
	return s.persist()
}

// SkillEnabled 读一个技能的开关台账；没有记录时 ok 为 false。
func (s *Store) SkillEnabled(skillDirName string) (enabled bool, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.SkillToggles[skillDirName]
	return v, ok
}

// SetSkillEnabled 写一个技能的开关台账并落盘。
func (s *Store) SetSkillEnabled(skillDirName string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.SkillToggles[skillDirName] = enabled
	return s.persist()
}
