package domain

// PromptTemplateVO 是一条提示词模板。
type PromptTemplateVO struct {
	// TemplateName 模板名（用 /name 触发，全局唯一）
	TemplateName string `json:"template_name"`
	// TemplateDesc 一句话说明（斜杠菜单里显示）
	TemplateDesc string `json:"template_desc"`
	// TemplateBody 正文：选中后填进输入框的提示词
	TemplateBody string `json:"template_body"`
	// TemplateArgs 参数占位提示（如 "<文件> <风格>"）；可空
	TemplateArgs string `json:"template_args"`
}

// PromptTemplateREQ 是保存一条模板的入参。
type PromptTemplateREQ struct {
	// TemplateName 模板名（小写字母/数字/连字符）
	TemplateName string `json:"template_name"`
	// TemplateDesc 一句话说明
	TemplateDesc string `json:"template_desc"`
	// TemplateBody 正文
	TemplateBody string `json:"template_body"`
	// TemplateArgs 参数占位提示
	TemplateArgs string `json:"template_args"`
}

// TemplateListRESP 是模板清单。
type TemplateListRESP struct {
	// Rows 模板行
	Rows []PromptTemplateVO `json:"rows"`
}
