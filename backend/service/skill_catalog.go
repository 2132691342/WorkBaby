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

// BuiltinSkills 是内置技能清单。筛选口径：只收与编程助手日常相关、
// 不依赖特定商业账号的技能——技能一旦落位就进系统提示，纯噪音不进来。
// 默认开关的判据是「沙箱里跑不跑得起来」：依赖 .NET SDK、gh CLI 这类
// 随包运行时里没有的东西的技能默认停用，装了也是进系统提示当噪音。
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
	{SkillDirName: "github", SkillGroup: "开发", SkillDefaultOn: false, SkillRequires: "gh CLI"},
	{SkillDirName: "全栈开发", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "前端开发", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "Impeccable（前端设计工具集）", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "mcp-builder", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "deep-research", SkillGroup: "研究", SkillDefaultOn: true, SkillRequires: "联网搜索"},
	{SkillDirName: "canvas-design", SkillGroup: "文档产出", SkillDefaultOn: true},
	{SkillDirName: "markitdown-skill", SkillGroup: "文档产出", SkillDefaultOn: false, SkillRequires: "Python markitdown"},
	{SkillDirName: "md-to-pdf-cjk", SkillGroup: "文档产出", SkillDefaultOn: true, SkillRequires: "Python reportlab"},
	{SkillDirName: "minimax-docx", SkillGroup: "文档产出", SkillDefaultOn: false, SkillRequires: ".NET SDK"},
	{SkillDirName: "minimax-pdf", SkillGroup: "文档产出", SkillDefaultOn: true, SkillRequires: "Python reportlab / playwright"},
	{SkillDirName: "minimax-xlsx", SkillGroup: "文档产出", SkillDefaultOn: false, SkillRequires: "LibreOffice（部分功能）"},
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
