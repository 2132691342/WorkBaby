package service

// RecommendedSkill 是一个随壳分发的推荐技能描述。
// 推荐不等于预置：技能目录要用户亲自点了才落位，
// 壳只在技能页摆出来，用户点了才复制进去。
type RecommendedSkill struct {
	// SkillDirName 目录名（与 assets\skills 下一致，是技能的身份）
	SkillDirName string
	// SkillGroup 界面上的分类标签
	SkillGroup string
	// SkillRequires 外部依赖说明；需要外部程序时在界面上点明
	SkillRequires string
}

// RecommendedSkills 是推荐技能清单。筛选口径：只收「日常用得上 + 不依赖奇怪外部程序」的。
// 技能一旦落位就进系统提示，纯噪音技能不进来。
var RecommendedSkills = []RecommendedSkill{
	{SkillDirName: "skill-creator", SkillGroup: "元能力"},
	{SkillDirName: "skill-vetter", SkillGroup: "元能力"},
	{SkillDirName: "diagnose", SkillGroup: "任务流程"},
	{SkillDirName: "tdd", SkillGroup: "任务流程"},
	{SkillDirName: "handoff", SkillGroup: "任务流程"},
	{SkillDirName: "Impeccable（前端设计工具集）", SkillGroup: "开发"},
	{SkillDirName: "deep-research", SkillGroup: "研究", SkillRequires: "联网搜索"},
	{SkillDirName: "canvas-design", SkillGroup: "文档产出"},
	{SkillDirName: "md-to-pdf-cjk", SkillGroup: "文档产出", SkillRequires: "Python reportlab"},
	{SkillDirName: "minimax-xlsx", SkillGroup: "文档产出", SkillRequires: "Python openpyxl"},
}

// RecommendedSkillOf 按目录名取推荐描述；不在清单里给 nil。
func RecommendedSkillOf(dirName string) *RecommendedSkill {
	for i := range RecommendedSkills {
		if RecommendedSkills[i].SkillDirName == dirName {
			return &RecommendedSkills[i]
		}
	}
	return nil
}
