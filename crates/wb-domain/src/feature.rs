//! 内置功能视图（12 项，替代旧插件面板）。

use serde::{Deserialize, Serialize};

/// 内置功能视图。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct FeatureVO {
    /// 功能码（todo / ask_user / memory / web / context_mode / fff / lsp / plan / goal /
    /// loadout / inspect / context_view）
    pub feature_code: String,
    /// 显示名 key（前端 i18n）
    pub feature_name: String,
    /// 一句话职责
    pub feature_description: String,
    /// 开关
    pub feature_enabled: bool,
    /// 提供的工具名清单
    pub feature_tools: Vec<String>,
    /// 阶段：p1 / p2（p2 功能在界面上标注「即将可用」）
    pub feature_phase: String,
    /// 是否有可配项（有则界面给配置入口）
    pub feature_configurable: bool,
}

/// 工具 / 技能开关的命名预设（loadout 功能）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct LoadoutPreset {
    /// 预设名（full / minimal 内置；其余用户自存）
    pub loadout_name: String,
    /// 启用的工具名集合
    pub loadout_tools: Vec<String>,
    /// 启用的技能名集合
    pub loadout_skills: Vec<String>,
}

/// 预设清单响应。
#[derive(Debug, Clone, Default, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct LoadoutListRESP {
    /// 内置 + 用户预设
    pub resp_presets: Vec<LoadoutPreset>,
}

/// 预设保存 / 应用请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct LoadoutREQ {
    /// 预设名
    pub loadout_name: String,
}

/// 配置字段类型：界面据此选择控件。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct FeatureConfigFieldVO {
    /// 字段名（与配置 JSON 的键一致）
    pub field_name: String,
    /// 显示名（界面文案 key 或原样显示）
    pub field_label: String,
    /// 一句话说明
    pub field_description: String,
    /// 控件类型：text / number / boolean / select / multi_select
    pub field_kind: String,
    /// 当前值（JSON 标量或数组）
    pub field_value: serde_json::Value,
    /// select / multi_select 的候选值
    pub field_options: Vec<FeatureConfigOptionVO>,
}

/// 配置字段的一个候选值。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct FeatureConfigOptionVO {
    /// 值
    pub option_value: String,
    /// 显示名
    pub option_label: String,
    /// 不可用原因（如缺密钥 / 未安装）；空 = 可选
    pub option_reason: String,
}

/// 功能配置响应：字段表 + 该功能是否提供配置。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct FeatureConfigRESP {
    /// 功能码
    pub resp_feature_code: String,
    /// 是否有可配项
    pub resp_configurable: bool,
    /// 字段清单
    pub resp_fields: Vec<FeatureConfigFieldVO>,
}

/// 功能配置保存请求：键 → 值（缺的键保持原值）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct FeatureConfigSaveREQ {
    /// 字段名 → 新值
    pub config_values: std::collections::BTreeMap<String, serde_json::Value>,
}
