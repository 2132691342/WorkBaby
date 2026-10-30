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
    /// 是否有功能级配置页
    pub feature_configurable: bool,
    /// 提供的工具名清单
    pub feature_tools: Vec<String>,
    /// 阶段：p1 / p2（p2 功能在界面上标注「即将可用」）
    pub feature_phase: String,
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
