//! 用量统计类型。

use serde::{Deserialize, Serialize};

/// 统计查询响应（GET /stats）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct StatsRESP {
    /// 按天聚合
    pub resp_daily: Vec<UsageDailyDO>,
    /// 按模型聚合
    pub resp_by_model: Vec<UsageModelDO>,
}

/// 天粒度用量 DO（usage_daily 表行）。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct UsageDailyDO {
    /// 日期（yyyy-MM-dd，本机时区）
    pub daily_day: String,
    /// 输入 token
    pub daily_input: i64,
    /// 输出 token
    pub daily_output: i64,
    /// 缓存读
    pub daily_cache_read: i64,
    /// 缓存写
    pub daily_cache_write: i64,
    /// 成本（美元）
    pub daily_cost: f64,
}

/// 模型粒度用量 DO。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct UsageModelDO {
    /// provider id
    pub model_provider: String,
    /// 模型 id
    pub model_id: String,
    /// 输入 token
    pub model_input: i64,
    /// 输出 token
    pub model_output: i64,
    /// 成本（美元）
    pub model_cost: f64,
}
