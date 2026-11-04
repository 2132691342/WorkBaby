package service

// BuiltinSkill 是一个随壳分发的技能描述。
type BuiltinSkill struct {
	// SkillDirName 目录名（与 assets\skills 下一致，是技能的身份）
	SkillDirName string
	// SkillGroup 界面上的分类标签
	SkillGroup string
	// SkillDefaultOn 是否默认启用
	SkillDefaultOn bool
	// SkillRequires 外部依赖说明；需要外部程序时在界面上点明
	SkillRequires string
}

// BuiltinSkills 是内置技能清单。
//
// 筛选口径：只收「与编程助手日常相关、且不依赖特定商业账号」的技能。
// 市场里那几百个垂直技能（订票、点外卖、查股票、公众号排版）不进来——
// 它们会把技能列表变成一片噪音，而技能一旦默认落位就会进系统提示。
var BuiltinSkills = []BuiltinSkill{
	{SkillDirName: "skill-creator", SkillGroup: "元能力", SkillDefaultOn: true},
	{SkillDirName: "skill-scanner", SkillGroup: "元能力", SkillDefaultOn: true},
	{SkillDirName: "skill-vetter", SkillGroup: "元能力", SkillDefaultOn: true},
	{SkillDirName: "prompt-engineering-expert", SkillGroup: "元能力", SkillDefaultOn: true},
	{SkillDirName: "task-alignment", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "task-implement", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "tdd", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "diagnose", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "handoff", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "setup-pre-commit", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "github", SkillGroup: "开发", SkillDefaultOn: true, SkillRequires: "gh CLI"},
	{SkillDirName: "全栈开发", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "前端开发", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "Impeccable（前端设计工具集）", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "mcp-builder", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "deep-research", SkillGroup: "研究", SkillDefaultOn: true, SkillRequires: "联网搜索"},
	{SkillDirName: "canvas-design", SkillGroup: "文档产出", SkillDefaultOn: true},
	{SkillDirName: "markitdown-skill", SkillGroup: "文档产出", SkillDefaultOn: false, SkillRequires: "Python markitdown"},
	{SkillDirName: "md-to-pdf-cjk", SkillGroup: "文档产出", SkillDefaultOn: true, SkillRequires: "Python reportlab"},
	{SkillDirName: "minimax-docx", SkillGroup: "文档产出", SkillDefaultOn: true, SkillRequires: ".NET SDK"},
	{SkillDirName: "minimax-pdf", SkillGroup: "文档产出", SkillDefaultOn: true},
	{SkillDirName: "minimax-xlsx", SkillGroup: "文档产出", SkillDefaultOn: false},
}

// BuiltinSkillOf 按目录名取内置描述；非内置给 nil。
func BuiltinSkillOf(dirName string) *BuiltinSkill {
	for i := range BuiltinSkills {
		if BuiltinSkills[i].SkillDirName == dirName {
			return &BuiltinSkills[i]
		}
	}
	return nil
}
