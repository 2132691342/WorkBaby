//! Provider / 模型类型：列表视图、编辑入参、models.json 外部契约（camelCase）。

use serde::{Deserialize, Serialize};

/// 模型视图。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ModelVO {
    /// 模型 id（provider 内唯一）
    pub model_id: String,
    /// 显示名
    pub model_name: String,
    /// 是否支持推理（thinking）
    pub model_reasoning: bool,
    /// 是否支持图片输入
    pub model_vision: bool,
    /// 上下文窗口
    pub model_context_window: i64,
    /// 最大输出
    pub model_max_tokens: i64,
}

/// 服务商视图（密钥只出掩码）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ProviderVO {
    /// models.json 里的键
    pub provider_id: String,
    /// 显示名
    pub provider_name: String,
    /// wire api：openai-completions / anthropic-messages / google-generative-ai
    pub provider_api: String,
    /// 接口地址
    pub provider_base_url: String,
    /// 是否已配密钥
    pub provider_has_key: bool,
    /// 掩码提示：sk-1••••abcd
    pub provider_key_hint: String,
    /// 模型清单
    pub provider_models: Vec<ModelVO>,
    /// 是否是默认服务商
    pub provider_is_default: bool,
}

/// 服务商清单响应。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ProviderListRESP {
    /// 全部服务商
    pub resp_providers: Vec<ProviderVO>,
    /// 默认 provider id
    pub resp_default_provider: String,
    /// 默认模型 id
    pub resp_default_model: String,
    /// models.json 路径
    pub resp_config_path: String,
}

/// 模型编辑入参。
#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ModelREQ {
    /// 模型 id
    pub model_id: String,
    /// 显示名
    pub model_name: String,
    /// 推理支持
    pub model_reasoning: bool,
    /// 图片输入
    pub model_vision: bool,
    /// 上下文窗口
    pub model_context_window: i64,
    /// 最大输出
    pub model_max_tokens: i64,
}

/// 服务商新增/编辑。
#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct UpsertProviderREQ {
    /// provider 键；空 = 新建（键取 provider_name）
    pub req_provider_id: Option<String>,
    /// 显示名
    pub req_provider_name: String,
    /// wire api
    pub req_provider_api: String,
    /// 接口地址
    pub req_provider_base_url: String,
    /// 密钥；空 = 不改动
    pub req_provider_api_key: Option<String>,
    /// 模型清单
    pub req_models: Vec<ModelREQ>,
}

/// 连通测试响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct TestProviderRESP {
    /// 是否连通
    pub resp_ok: bool,
    /// 实际探测的模型
    pub resp_model: String,
    /// 失败原因或成功提示
    pub resp_message: String,
}

/// 密钥明文查看响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct RevealProviderRESP {
    /// provider 键
    pub resp_provider_id: String,
    /// 密钥明文
    pub resp_api_key: String,
}

/// 可选模型（跨 provider 平铺）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct AvailableModelVO {
    /// 模型 id
    pub model_id: String,
    /// 显示名
    pub model_name: String,
    /// 所属 provider 键
    pub model_provider: String,
    /// 上下文窗口
    pub model_context_window: i64,
    /// 最大输出
    pub model_max_tokens: i64,
    /// 推理支持
    pub model_reasoning: bool,
    /// 图片输入
    pub model_vision: bool,
}

// ---- models.json 外部契约（camelCase，与前端面板直读）----

/// models.json 里的模型条目。
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase", default)]
pub struct ModelCfg {
    /// 模型 id
    pub id: String,
    /// 显示名
    pub name: String,
    /// 推理支持
    pub reasoning: bool,
    /// 图片输入
    pub vision: bool,
    /// 上下文窗口
    pub context_window: i64,
    /// 最大输出
    pub max_tokens: i64,
}

/// models.json 里的 provider 条目。
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase", default)]
pub struct ProviderCfg {
    /// 显示名
    pub name: String,
    /// 接口地址
    pub base_url: String,
    /// wire api
    pub api: String,
    /// 密钥（字面量或 $ENV）
    pub api_key: String,
    /// 附加头
    pub headers: std::collections::BTreeMap<String, String>,
    /// 模型清单
    pub models: Vec<ModelCfg>,
}

/// models.json 顶层。
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase", default)]
pub struct ModelsCfg {
    /// provider 键值表
    pub providers: std::collections::BTreeMap<String, ProviderCfg>,
}

/// auth.json 单条凭据。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum AuthEntry {
    /// API key 凭据
    ApiKey {
        /// 密钥本体
        key: String,
    },
    /// OAuth 凭据
    OAuth {
        /// 刷新令牌
        refresh: String,
        /// 访问令牌
        access: String,
        /// 过期毫秒
        expires: i64,
    },
}

/// auth.json 顶层。
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct AuthCfg {
    /// provider 键 → 凭据
    pub providers: std::collections::BTreeMap<String, AuthEntry>,
}
