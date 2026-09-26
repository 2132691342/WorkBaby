package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/resource"
)

// AgentFileDir 用户级子智能体定义目录（{home}/agents）。
func AgentFileDir(home string) string { return filepath.Join(home, "agents") }

// agentFileFields 定义文件支持但当前未接入执行链的字段。
// 列出来是为了「写错不生效」能被告警，而不是静默当没写。
var agentFileFields = []string{"color", "mcpServers", "injectAgentsMd"}

// thinkingLevels 允许的推理强度取值（与 llm.ThinkingFromEffort 一致）。
var thinkingLevels = map[string]struct{}{"off": {}, "low": {}, "medium": {}, "high": {}}

// LoadAgentFiles 读取用户级子智能体定义文件（frontmatter + 正文人设），与设置页创建的
// 智能体同构、最终都物化成 agent.Definition（文件可随 dotfiles 分发，表适合界面里随手改）。
// 缺 name / description、名字非法、与内置名撞名的文件诊断后跳过。
func LoadAgentFiles(home string) []agent.Definition {
	if strings.TrimSpace(home) == "" {
		return nil
	}
	dir := AgentFileDir(home)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	builtin := map[string]struct{}{}
	for _, name := range agent.BuiltinAgentNames() {
		builtin[name] = struct{}{}
	}
	out := make([]agent.Definition, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		file := e.Name()
		data, rerr := os.ReadFile(filepath.Join(dir, file))
		if rerr != nil {
			pkg.L.Warn("agent file read failed", "file", file, "err", rerr.Error())
			continue
		}
		def, ok := parseAgentFile(data, file, builtin)
		if !ok {
			continue
		}
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// parseAgentFile 解析单个 Agent 定义文件。
func parseAgentFile(data []byte, file string, builtin map[string]struct{}) (agent.Definition, bool) {
	fm := resource.SplitFrontMatter(string(data))
	// 文件名是缺省名；frontmatter 里的 name 优先（与定义文件的自述一致）
	name := strings.TrimSpace(fm.Get("name"))
	if name == "" {
		name = strings.TrimSuffix(file, filepath.Ext(file))
	}
	desc := strings.TrimSpace(fm.Get("description"))
	if name == "" || desc == "" {
		pkg.L.Warn("agent file skipped: name/description required", "file", file)
		return agent.Definition{}, false
	}
	if !agentNamePattern.MatchString(name) {
		pkg.L.Warn("agent file skipped: invalid name", "file", file, "name", name)
		return agent.Definition{}, false
	}
	if _, dup := builtin[name]; dup {
		pkg.L.Warn("agent file skipped: name reserved by builtin agent", "file", file, "name", name)
		return agent.Definition{}, false
	}
	if unsupported := unsupportedAgentFields(fm); len(unsupported) > 0 {
		pkg.L.Warn("agent file: unsupported fields ignored",
			"agent", name, "fields", strings.Join(unsupported, ","))
	}
	tools := fm.List("tools")
	allow := tools
	if len(allow) == 1 && (allow[0] == "*" || allow[0] == "all") {
		allow = nil // 显式「全部工具」= 不设白名单
	}
	maxTurns := fm.IntValue("maxTurns", 0)
	model := strings.TrimSpace(fm.Get("model"))
	// 继承主模型的写法与「不写」等价
	if model == "inherit" {
		model = ""
	}
	thinking := strings.ToLower(strings.TrimSpace(fm.Get("thoughtLevel")))
	if thinking != "" {
		if _, ok := thinkingLevels[thinking]; !ok {
			pkg.L.Warn("agent file: invalid thoughtLevel ignored", "agent", name, "value", thinking)
			thinking = ""
		} else if model == "" {
			// 与「子智能体的推理强度仅在指定了具体模型时生效」一致：
			// 不指定模型 = 完全跟随会话，单方面改思考档位会让用户当次的选择失效。
			pkg.L.Warn("agent file: thoughtLevel ignored without model", "agent", name, "value", thinking)
			thinking = ""
		}
	}
	return agent.Definition{
		Name:        name,
		Description: desc,
		Persona:     strings.TrimSpace(fm.Body()),
		Tools: agent.ToolPolicy{
			Allow: allow,
			Deny:  fm.List("disallowedTools"),
		},
		// 子智能体默认不读写长期记忆：主对话的记忆属于主对话，子任务污染召回会降低准确率。
		Memory:   agent.MemoryPolicy{Enabled: false, Formation: false},
		Budget:   agent.Budget{MaxTurns: maxTurns},
		Model:    model,
		Thinking: thinking,
	}, true
}

// unsupportedAgentFields 返回定义文件里出现但当前未接入执行链的字段。
func unsupportedAgentFields(fm resource.FrontMatter) []string {
	var out []string
	for _, k := range agentFileFields {
		if fm.Get(k) != "" {
			out = append(out, k)
		}
	}
	return out
}
