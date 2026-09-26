package agent

import (
	"path"
	"strings"
	"time"

	"WorkBaby/internal/llm"
)

// Definition 声明式 Agent 定义：人设 / 工具策略 / 运行预算。纯数据，无依赖注入。
//
// 同一定义可在 chat / 子 Agent 委派 / 后台任务中复用，替换各调用点的硬编码参数。
type Definition struct {
	Name        string       // 唯一名（"default" / "explore"）
	Description string       // 一句话职责（供界面选择与主 Agent 判断何时委派）
	Persona     string       // 人设与方法论；空 = 不注入
	Tools       ToolPolicy   // 工具策略
	Memory      MemoryPolicy // 记忆策略
	Budget      Budget       // 运行预算
	Model       string       // 专属模型；空 = 跟随会话模型
	Thinking    string       // 推理强度 off/low/medium/high；空 = 跟随请求与全局设置
}

// MemoryPolicy 记忆接入策略。
type MemoryPolicy struct {
	Enabled     bool // 是否参与长期记忆召回
	RecallLimit int  // 召回条数上限；<=0 用默认
	Formation   bool // run 完成后是否形成记忆
}

// EffectiveModel 本次 run 实际使用的模型：定义优先，否则跟随会话。
func (d Definition) EffectiveModel(sessionModel string) string {
	if strings.TrimSpace(d.Model) != "" {
		return d.Model
	}
	return sessionModel
}

// PersonaSection 人设注入段；空人设返回零值（BuildSystem 会跳过空段）。
func (d Definition) PersonaSection() Section {
	return Section{Title: "角色", Body: d.Persona, Order: OrderPersona}
}

// ToolPolicy 工具白名单/黑名单（glob）。
// 与执行层的风险裁决分工：这里声明「该 Agent 带哪些工具」，审批与沙箱在护栏中间件里。
type ToolPolicy struct {
	Allow    []string // 白名单；空 = 全部已暴露工具
	Deny     []string // 黑名单
	MaxTools int      // 暴露数量上限；<=0 不限制
}

// Budget 运行预算。零值表示不限制，由调用方补默认。
type Budget struct {
	MaxTurns        int           // 轮次上限
	ContextTokens   int           // 单轮消息估算 token 预算；超预算触发压缩
	ToolCallTimeout time.Duration // 单次工具执行超时
	MaxWallTime     time.Duration // 整个 run 的墙钟上限（由调用方作用到 ctx）
}

// Apply 把非零预算落到 Loop 配置。
func (b Budget) Apply(cfg *Config) {
	if cfg == nil {
		return
	}
	if b.MaxTurns > 0 {
		cfg.MaxTurns = b.MaxTurns
	}
	if b.ContextTokens > 0 {
		cfg.MaxInput = b.ContextTokens
	}
	if b.ToolCallTimeout > 0 {
		cfg.Exec.Timeout = b.ToolCallTimeout
	}
}

// FilterTools 按策略过滤工具定义：Deny 命中剔除 → Allow 非空仅留命中（glob）→ MaxTools 截断。
func (d Definition) FilterTools(defs []llm.ToolDefinition) []llm.ToolDefinition {
	if len(d.Tools.Allow) == 0 && len(d.Tools.Deny) == 0 && d.Tools.MaxTools <= 0 {
		return defs
	}
	out := make([]llm.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if matchAnyGlob(def.Name, d.Tools.Deny) {
			continue
		}
		if len(d.Tools.Allow) > 0 && !matchAnyGlob(def.Name, d.Tools.Allow) {
			continue
		}
		out = append(out, def)
	}
	if d.Tools.MaxTools > 0 && len(out) > d.Tools.MaxTools {
		out = out[:d.Tools.MaxTools]
	}
	return out
}

// ExposedNames 取出过滤后的工具名清单，供 Loop.Expose 使用。
func (d Definition) ExposedNames(all []llm.ToolDefinition) []string {
	filtered := d.FilterTools(all)
	if len(d.Tools.Allow) == 0 && len(d.Tools.Deny) == 0 && d.Tools.MaxTools <= 0 {
		return nil // 不设白名单：让注册表全量暴露，避免把「未列出」误判为「不暴露」
	}
	names := make([]string, 0, len(filtered))
	for _, def := range filtered {
		names = append(names, def.Name)
	}
	return names
}

// matchAnyGlob 判断 name 命中任一 glob（path.Match 语义）。
func matchAnyGlob(name string, pats []string) bool {
	for _, p := range pats {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// personaMethodology 通用工作方法论；内置 Agent 共用。
// 每条都是可判定的动作约束——空泛的「先理解再行动」无法指导行为。
const personaMethodology = `
## 工作原则
1. 先探查再动手：信息不足时先用 file_list / file_read / doc_reader 看清现状，不凭猜测下结论。
2. 多步任务先列计划：开工前用 todo(plan) 拆成可勾选的步骤，每完成一项立即 todo(mark_done)。
3. 最小改动：只动与目标直接相关的部分，不顺手重构，不臆造不存在的 API、路径或参数。
4. 用工具验证结果：写完代码就 exec 跑构建或测试，改完文件读回来确认——不要凭记忆断言「已完成」。
5. 失败要改道，不要硬重试：同一调用连续失败两次就换工具、换参数或向用户说明，禁止原样重复。
6. 不确定就问：关键前提缺失且无法自行推断时，向用户确认，不要替用户假设。
7. 成果必须可核验：严禁声称「已创建/已生成/已保存」任何文件或「已执行」任何操作，除非本轮确实调用了对应工具且成功。

## 输出
- 简洁中文，先结论后过程；不复述工具返回的原文，只给结论与必要证据。
- 任务收尾时明确列出：改了哪些文件、跑了什么命令、结果如何。`

// personaDefault 通用助手：本机可真实调用工具干活。
const personaDefault = "你是 WorkBaby，运行在用户本机上的个人 AI 助手，可以调用工具真实干活，而不只是给建议。" +
	personaMethodology

// personaExplore 只读探索：被主 Agent 派出去查事实，回传证据而非结论。
// 与默认人设的关键差别是「不产出改动」——写进人设，模型才不会先改再解释。
const personaExplore = "你是 WorkBaby 的只读探索专家，负责在隔离上下文里查清事实并把证据带回来。\n" +
	"工作方式：\n" +
	"1. 只读：不创建、不修改、不删除任何文件，也不执行会改变系统状态的命令。\n" +
	"2. 先铺开再收敛：先用 file_list / file_grep 定位范围，再 file_read 精读关键位置。\n" +
	"3. 结论必须带证据：每条结论标注「文件路径 + 行号区间」，便于直接复核。\n" +
	"4. 区分事实与推断：读到的写「事实」，没读到的写「未确认」。\n" +
	"5. 有界收束：围绕被指派的问题作答，不做无关的横向扩展。\n\n" +
	"## 输出\n" +
	"- 先给结论，再列证据清单（路径 + 行号 + 一句话说明）。\n" +
	"- 内容不足以回答时，明确说明缺什么、已查过哪些位置。"
