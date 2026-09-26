package resource

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// SlashCommandSourceFile 用户级命令文件来源（home/commands/*.md）。
const SlashCommandSourceFile = "file"

// SlashCommandSourceWorkspace 工作区级命令文件来源（<ws>/.workbaby/commands/*.md）。
const SlashCommandSourceWorkspace = "workspace"

// CommandFileDir 用户命令文件目录（{home}/commands）。
func CommandFileDir(home string) string { return filepath.Join(home, "commands") }

// WorkspaceCommandDir 工作区命令文件目录（<ws>/.workbaby/commands）；未绑定工作区返回空串。
// 与工作区技能目录（.workbaby/skills）同一约定：过程数据随工作区走，不污染用户项目根。
func WorkspaceCommandDir(wsPath string) string {
	if strings.TrimSpace(wsPath) == "" {
		return ""
	}
	return filepath.Join(wsPath, ".workbaby", "commands")
}

// LoadCommands 加载命令文件目录：用户级 + 工作区级，按名称归一合并，
// 工作区 > 用户级同名优先（同名命令工作区覆盖，与 skill 同约定）。
// 目录缺失静默跳过，文件写坏只跳过该条并告警。
func LoadCommands(home, wsPath string) []domain.SlashCommand {
	homeCmds := loadCommandDir(CommandFileDir(strings.TrimSpace(home)), SlashCommandSourceFile)
	wsCmds := loadCommandDir(WorkspaceCommandDir(wsPath), SlashCommandSourceWorkspace)
	// 工作区同名覆盖用户级
	byName := make(map[string]domain.SlashCommand, len(homeCmds)+len(wsCmds))
	for _, c := range homeCmds {
		byName[c.Name] = c
	}
	for _, c := range wsCmds {
		byName[c.Name] = c
	}
	out := make([]domain.SlashCommand, 0, len(byName))
	for _, c := range byName {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LoadCommandFiles 用户级命令文件（保留旧 API；等价于 LoadCommands(home, "") 取用户级）。
func LoadCommandFiles(home string) []domain.SlashCommand {
	return loadCommandDir(CommandFileDir(strings.TrimSpace(home)), SlashCommandSourceFile)
}

// LoadWorkspaceCommandFiles 工作区级命令文件（保留旧 API；等价于 LoadCommands("", wsPath) 取工作区级）。
func LoadWorkspaceCommandFiles(wsPath string) []domain.SlashCommand {
	return loadCommandDir(WorkspaceCommandDir(wsPath), SlashCommandSourceWorkspace)
}

// loadCommandDir 读取一个命令目录。
func loadCommandDir(dir, source string) []domain.SlashCommand {
	if dir == "" {
		return nil
	}
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
		if !CommandNamePattern.MatchString(name) {
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
	return out
}

// parseCommandFile 解析单个命令文件：frontmatter 给元数据，正文即提示词模板
// （可含 $ARGUMENTS 与 $1..$n 占位，由发送前展开）。
func parseCommandFile(data []byte, name string) (domain.SlashCommand, bool) {
	fm := SplitFrontMatter(string(data))
	if strings.TrimSpace(fm.Body()) == "" {
		pkg.L.Warn("command file skipped: empty prompt", "command", name)
		return domain.SlashCommand{}, false
	}
	// 这三类字段属于「按命令收窄权限 / 换模型」的扩展点，当前命令模型只承载提示词模板；
	// 显式告警而不是静默忽略——写了不生效的配置比没有配置更难排查。
	if v := []string{"allowed-tools", "model", "skills"}; fm.HasAnyField(v) {
		pkg.L.Warn("command file: unsupported fields ignored",
			"command", name, "fields", strings.Join(unsupportedFields(fm, v), ","))
	}
	return domain.SlashCommand{
		Name:       name,
		Args:       fm.Get("argument-hint"),
		Desc:       fm.Get("description"),
		Group:      "custom",
		ClientOnly: true,
		Prompt:     fm.Body(),
	}, true
}