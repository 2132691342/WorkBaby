package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/runtime"
)

// CommandFileDir 用户命令文件目录（{home}/commands）。
func CommandFileDir(home string) string { return filepath.Join(home, "commands") }

// WorkspaceCommandDir 工作区命令文件目录（<ws>/.workbaby/commands）；未绑定工作区返回空串。
// 与工作区技能目录（.workbaby/skills）同一约定：过程数据随工作区走，不污染用户项目根。
func WorkspaceCommandDir(wsPath string) string {
	if strings.TrimSpace(wsPath) == "" {
		return ""
	}
	return filepath.Join(wsPath, runtime.SandboxDirName, "commands")
}

// LoadCommandFiles 读取用户级命令文件目录，物化为斜杠命令（group=custom、选中即灌入输入框）。
//
// 文件是命令的「工程化」载体：可随 dotfiles 同步、可用 Git 管理、不必进设置页逐条录。
// 同名优先序由调用方决定（工作区 > 用户级 > 设置页）。
// 目录不存在、文件写坏、命令名非法都只跳过该文件并告警，不影响其余命令。
func LoadCommandFiles(home string) []domain.SlashCommand {
	if strings.TrimSpace(home) == "" {
		return nil
	}
	return loadCommandDir(CommandFileDir(home), domain.CommandSourceFile)
}

// LoadWorkspaceCommandFiles 读取当前会话工作区下的命令文件；未绑定工作区返回 nil。
// 工作区级命令面向「这个项目才有的流程」（组内约定、仓库专属脚本），随仓库分发到同组成员。
func LoadWorkspaceCommandFiles(wsPath string) []domain.SlashCommand {
	dir := WorkspaceCommandDir(wsPath)
	if dir == "" {
		return nil
	}
	return loadCommandDir(dir, domain.CommandSourceWorkspace)
}

// loadCommandDir 读取一个命令目录（用户级 / 工作区级共用）。
func loadCommandDir(dir, source string) []domain.SlashCommand {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]domain.SlashCommand, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if !commandNamePattern.MatchString(name) {
			pkg.L.Warn("command file skipped: invalid name", "dir", dir, "file", e.Name())
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			pkg.L.Warn("command file read failed", "file", e.Name(), "err", rerr.Error())
			continue
		}
		if cmd, ok := parseCommandFile(data, name); ok {
			cmd.Source = source
			out = append(out, cmd)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// parseCommandFile 解析单个命令文件：frontmatter 给元数据，正文即提示词模板
// （可含 $ARGUMENTS 与 $1..$n 占位，由发送前展开）。
func parseCommandFile(data []byte, name string) (domain.SlashCommand, bool) {
	fm := splitFrontMatter(string(data))
	if strings.TrimSpace(fm.body) == "" {
		pkg.L.Warn("command file skipped: empty prompt", "command", name)
		return domain.SlashCommand{}, false
	}
	// 这三类字段属于「按命令收窄权限 / 换模型」的扩展点，当前命令模型只承载提示词模板；
	// 显式告警而不是静默忽略——写了不生效的配置比没有配置更难排查。
	if v := []string{"allowed-tools", "model", "skills"}; hasAnyField(fm, v) {
		pkg.L.Warn("command file: unsupported fields ignored",
			"command", name, "fields", strings.Join(unsupportedFields(fm, v), ","))
	}
	return domain.SlashCommand{
		Name:       name,
		Args:       fm.get("argument-hint"),
		Desc:       fm.get("description"),
		Group:      "custom",
		ClientOnly: true,
		Prompt:     fm.body,
	}, true
}

// hasAnyField 任一字段存在即真。
func hasAnyField(fm frontMatter, keys []string) bool { return len(unsupportedFields(fm, keys)) > 0 }

// unsupportedFields 返回实际出现但当前不生效的字段名。
func unsupportedFields(fm frontMatter, keys []string) []string {
	var out []string
	for _, k := range keys {
		if fm.get(k) != "" {
			out = append(out, k)
		}
	}
	return out
}
