//! models.json 读写与解析后的可请求形态。

use std::path::{Path, PathBuf};

use wb_domain::provider::{AuthCfg, ModelsCfg};
use wb_pkg::fsutil;
use wb_pkg::{AppError, Result};

/// 解析后可发请求的 provider。
#[derive(Debug, Clone)]
pub struct ResolvedProvider {
    /// provider 键
    pub provider_id: String,
    /// 接口地址（去尾斜杠）
    pub base_url: String,
    /// wire api
    pub api: String,
    /// 解析后的密钥（$ENV 已展开；空串 = 未配）
    pub api_key: String,
    /// 附加头
    pub headers: Vec<(String, String)>,
    /// 模型 id
    pub model_id: String,
    /// 上下文窗口
    pub context_window: i64,
    /// 最大输出
    pub max_tokens: i64,
}

/// models.json + auth.json 的组合存储。
pub struct ProviderStore {
    /// models.json 路径
    pub models_path: PathBuf,
    /// auth.json 路径
    pub auth_path: PathBuf,
}

impl ProviderStore {
    /// 由数据目录构造。
    pub fn new(data_dir: &Path) -> Self {
        Self {
            models_path: data_dir.join("models.json"),
            auth_path: data_dir.join("auth.json"),
        }
    }

    /// 读 models.json；不存在返回空表。
    pub fn load_models(&self) -> Result<ModelsCfg> {
        if !self.models_path.exists() {
            return Ok(ModelsCfg::default());
        }
        let text = fsutil::read_to_string(&self.models_path)?;
        serde_json::from_str(&text).map_err(|e| AppError::wrap("2001", "models.json 解析失败", e))
    }

    /// 写 models.json（原子写）。
    pub fn save_models(&self, cfg: &ModelsCfg) -> Result<()> {
        let text = serde_json::to_string_pretty(cfg)
            .map_err(|e| AppError::wrap("2001", "序列化失败", e))?;
        fsutil::write_atomic(&self.models_path, text.as_bytes())
    }

    /// 读 auth.json；不存在返回空表。
    pub fn load_auth(&self) -> Result<AuthCfg> {
        if !self.auth_path.exists() {
            return Ok(AuthCfg::default());
        }
        let text = fsutil::read_to_string(&self.auth_path)?;
        serde_json::from_str(&text).map_err(|e| AppError::wrap("2001", "auth.json 解析失败", e))
    }

    /// 写 auth.json（原子写）。
    pub fn save_auth(&self, cfg: &AuthCfg) -> Result<()> {
        let text = serde_json::to_string_pretty(cfg)
            .map_err(|e| AppError::wrap("2001", "序列化失败", e))?;
        fsutil::write_atomic(&self.auth_path, text.as_bytes())
    }

    /// 解析指定 provider + 模型为可请求形态；缺模型走该 provider 首个。
    pub fn resolve(&self, provider_id: &str, model_id: Option<&str>) -> Result<ResolvedProvider> {
        let models = self.load_models()?;
        let cfg = models.providers.get(provider_id).ok_or_else(|| {
            AppError::new(wb_domain::error::provider::NOT_FOUND, "服务商不存在")
                .with_details(provider_id.to_string())
        })?;
        let chosen = match model_id {
            Some(m) if !m.is_empty() => m.to_string(),
            _ => cfg
                .models
                .first()
                .map(|m| m.id.clone())
                .ok_or_else(|| AppError::new("3002", "该服务商未配置任何模型"))?,
        };
        let model = cfg
            .models
            .iter()
            .find(|m| m.id == chosen)
            .ok_or_else(|| AppError::new("3002", "模型不存在").with_details(chosen.clone()))?;
        // 密钥级联：auth.json 优先（面板「保存密钥」落这里），其次配置里的 $ENV 展开值
        let auth = self.load_auth()?;
        let mut key = match auth.providers.get(provider_id) {
            Some(wb_domain::provider::AuthEntry::ApiKey { key }) => key.clone(),
            _ => String::new(),
        };
        if key.is_empty() {
            key = expand_key(&cfg.api_key);
        }
        let base_url = cfg.base_url.trim_end_matches('/').to_string();
        if base_url.is_empty() {
            return Err(AppError::new(
                wb_domain::error::provider::BASE_URL_INVALID,
                "接口地址未配置",
            ));
        }
        Ok(ResolvedProvider {
            provider_id: provider_id.to_string(),
            base_url,
            api: cfg.api.clone(),
            api_key: key,
            headers: cfg
                .headers
                .iter()
                .map(|(k, v)| (k.clone(), v.clone()))
                .collect(),
            model_id: chosen,
            context_window: if model.context_window > 0 {
                model.context_window
            } else {
                128_000
            },
            max_tokens: if model.max_tokens > 0 {
                model.max_tokens
            } else {
                16_384
            },
        })
    }
}

/// 展开 `$ENV_NAME` 形式的密钥；无前置 $ 原样返回。
fn expand_key(raw: &str) -> String {
    let Some(env_name) = raw.strip_prefix('$') else {
        return raw.to_string();
    };
    std::env::var(env_name).unwrap_or_default()
}
