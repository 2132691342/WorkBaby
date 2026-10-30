//! 设置与引导类型。

use serde::{Deserialize, Serialize};

/// 设置键值 DO（settings_kv 表）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SettingKV {
    /// 键
    pub setting_key: String,
    /// 值（字符串形态）
    pub setting_value: String,
}

/// 批量写设置请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SettingsSaveREQ {
    /// 键值对（单键 upsert 也走它）
    pub req_values: Vec<SettingKV>,
}

/// 引导数据（GET /bootstrap）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct BootVO {
    /// 应用版本
    pub boot_version: String,
    /// 数据目录
    pub boot_data_dir: String,
    /// 当前工作区
    pub boot_workspace: String,
    /// 内核是否就绪（就绪 = 已配默认模型）
    pub boot_ready: bool,
    /// 未就绪的阻塞原因（人话）
    pub boot_block_reason: String,
    /// 默认服务商（可空）
    pub boot_default_provider: String,
    /// 默认模型（可空）
    pub boot_default_model: String,
    /// 全部设置键值
    pub boot_settings: std::collections::BTreeMap<String, String>,
}
