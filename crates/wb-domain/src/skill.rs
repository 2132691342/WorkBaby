//! 技能类型：视图对象与 SKILL.md frontmatter（自由字段透传）。

use serde::{Deserialize, Serialize};

/// 技能来源三区。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SkillSource {
    /// 随应用分发的内置包
    Builtin,
    /// 数据目录用户技能
    User,
    /// 工作区 .workbaby/skills
    Workspace,
}

/// 技能视图。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SkillVO {
    /// 技能名（frontmatter name，缺省目录名）；身份即此名
    pub skill_name: String,
    /// 描述（≤1024）
    pub skill_description: String,
    /// 中文描述
    pub skill_description_zh: Option<String>,
    /// 英文描述
    pub skill_description_en: Option<String>,
    /// 版本
    pub skill_version: Option<String>,
    /// 来源三区
    pub skill_source: SkillSource,
    /// SKILL.md 绝对路径
    pub skill_location: String,
    /// 开关
    pub skill_enabled: bool,
}

/// SKILL.md frontmatter 解析产物：已知字段强类型，未知字段落 metadata。
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
pub struct SkillFrontmatter {
    /// 技能名
    pub name: Option<String>,
    /// 描述（支持 | 与 > 块标量）
    pub description: Option<String>,
    /// 中文描述
    pub description_zh: Option<String>,
    /// 英文描述
    pub description_en: Option<String>,
    /// 版本
    pub version: Option<String>,
    /// 许可
    pub license: Option<String>,
    /// 主页
    pub homepage: Option<String>,
    /// 作者
    pub author: Option<String>,
    /// 建议工具（逗号串或数组）
    pub allowed_tools: Option<serde_json::Value>,
    /// 触发词
    pub triggers: Option<serde_json::Value>,
    /// 关键词
    pub keywords: Option<serde_json::Value>,
    /// 自由嵌套元数据（原样保留）
    pub metadata: Option<serde_json::Value>,
    /// true 则不注入 available_skills，仅 /skill:name 可用
    pub disable_model_invocation: Option<bool>,
    /// 解析器保留的其余未知键
    #[serde(flatten)]
    pub extra: std::collections::BTreeMap<String, serde_json::Value>,
}

// ---- 技能管理 ----

/// 新建技能请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SkillCreateREQ {
    /// 技能名（即 id）
    pub req_skill_name: String,
    /// 描述
    pub req_skill_description: String,
    /// 正文（frontmatter 之外的 markdown）
    pub req_skill_body: String,
}

/// 导入结果。
#[derive(Debug, Clone, Default, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SkillImportRESP {
    /// 成功条数
    pub resp_imported: i64,
    /// 跳过的名字与原因
    pub resp_skipped: Vec<String>,
}

/// zip 导入请求（data 为 zip 字节的 base64）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SkillImportZipREQ {
    /// 文件名（仅展示）
    pub req_skill_filename: String,
    /// zip 原始字节的 base64
    pub req_skill_data: String,
}

/// 技能正文响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SkillContentRESP {
    /// SKILL.md 原文
    pub resp_content: String,
}
