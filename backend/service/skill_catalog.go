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

// BuiltinSkills 是内置技能清单。筛选口径：只收「沙箱里跑得起来 + 日常用得上」的——
// 依赖沙箱外程序（.NET SDK、gh CLI、playwright、LibreOffice）或与内置插件职责
// 重叠的一律不收；技能落位就进系统提示，纯噪音不进来。
var BuiltinSkills = []BuiltinSkill{
	{SkillDirName: "skill-creator", SkillGroup: "元能力", SkillDefaultOn: true},
	{SkillDirName: "skill-vetter", SkillGroup: "元能力", SkillDefaultOn: true},
	{SkillDirName: "diagnose", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "tdd", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "handoff", SkillGroup: "任务流程", SkillDefaultOn: true},
	{SkillDirName: "Impeccable（前端设计工具集）", SkillGroup: "开发", SkillDefaultOn: true},
	{SkillDirName: "deep-research", SkillGroup: "研究", SkillDefaultOn: true, SkillRequires: "联网搜索"},
	{SkillDirName: "canvas-design", SkillGroup: "文档产出", SkillDefaultOn: true},
	{SkillDirName: "md-to-pdf-cjk", SkillGroup: "文档产出", SkillDefaultOn: true, SkillRequires: "Python reportlab"},
	{SkillDirName: "minimax-xlsx", SkillGroup: "文档产出", SkillDefaultOn: false, SkillRequires: "Python openpyxl"},
}

// removedBuiltinSkills 是历史上内置过、现已下架的技能目录名。
// 落位时顺手清掉沙箱里的旧副本，清单缩编才能在老数据目录上真落地；
// 这些名字归内置清单管，用户后来自建的同名技能会被一并清掉，换的是列表不再积灰。
var removedBuiltinSkills = []string{
	"skill-scanner",
	"prompt-engineering-expert",
	"task-alignment",
	"task-implement",
	"setup-pre-commit",
	"github",
	"全栈开发",
	"前端开发",
	"mcp-builder",
	"markitdown-skill",
	"minimax-docx",
	"minimax-pdf",
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
