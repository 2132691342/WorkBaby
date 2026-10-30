//! Provider 管理：models.json / auth.json 的读写编排、连通测试、模型清单拉取。

use std::sync::Arc;

use wb_domain::error::provider as code;
use wb_domain::provider::{
    AvailableModelVO, ProviderCfg, ProviderVO, RevealProviderRESP, TestProviderRESP,
    UpsertProviderREQ,
};
use wb_pkg::{AppError, Result};

use super::Services;

/// 测试与拉清单的超时。
const HTTP_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(15);

/// 取指定 provider 的实际密钥（auth.json 优先，models.json 字面量兜底）。
fn effective_key(sv: &Services, pid: &str, cfg: &ProviderCfg) -> Result<String> {
    let auth = sv.providers.load_auth()?;
    if let Some(wb_domain::provider::AuthEntry::ApiKey { key }) = auth.providers.get(pid) {
        return Ok(key.clone());
    }
    Ok(cfg.api_key.clone())
}

/// 列表视图（掩码不泄露密钥）。
pub fn list_vo(sv: &Arc<Services>) -> Result<(Vec<ProviderVO>, String, String, String)> {
    let models = sv.providers.load_models()?;
    let auth = sv.providers.load_auth()?;
    let dp = sv
        .repo
        .settings_get("default_provider")?
        .unwrap_or_default();
    let dm = sv.repo.settings_get("default_model")?.unwrap_or_default();
    let mut out = Vec::new();
    for (pid, cfg) in &models.providers {
        let has_key = auth.providers.contains_key(pid) || !cfg.api_key.is_empty();
        let hint = if has_key {
            let chars: Vec<char> = cfg.api_key.chars().collect();
            let tail: String = chars
                .iter()
                .rev()
                .take(4)
                .collect::<String>()
                .chars()
                .rev()
                .collect();
            format!("••••{tail}")
        } else {
            String::new()
        };
        out.push(ProviderVO {
            provider_id: pid.clone(),
            provider_name: cfg.name.clone(),
            provider_api: cfg.api.clone(),
            provider_base_url: cfg.base_url.clone(),
            provider_has_key: has_key,
            provider_key_hint: hint,
            provider_models: cfg
                .models
                .iter()
                .map(|m| wb_domain::provider::ModelVO {
                    model_id: m.id.clone(),
                    model_name: m.name.clone(),
                    model_reasoning: m.reasoning,
                    model_vision: m.vision,
                    model_context_window: m.context_window,
                    model_max_tokens: m.max_tokens,
                })
                .collect(),
            provider_is_default: *pid == dp,
        });
    }
    Ok((out, dp, dm, sv.providers.models_path.display().to_string()))
}

/// 新增 / 编辑：键空 = 新建（键取名字），密钥空 = 不改动。
pub async fn upsert(sv: &Arc<Services>, req: &UpsertProviderREQ) -> Result<String> {
    let key = req.req_provider_id.clone().unwrap_or_default();
    let pid = if key.is_empty() {
        req.req_provider_name.trim().to_string()
    } else {
        key
    };
    if pid.is_empty() || pid.contains(['/', '\\', ':']) {
        return Err(AppError::new(code::NAME_DUP, "服务商名非法"));
    }
    let mut models = sv.providers.load_models()?;
    if !models.providers.contains_key(&pid) && req.req_provider_id.is_none() {
        // 新建时重名检查：名字即键，重复直接报
        if models
            .providers
            .values()
            .any(|c| c.name == req.req_provider_name)
        {
            return Err(AppError::new(
                code::NAME_DUP,
                format!("已存在同名服务商 {}", req.req_provider_name),
            ));
        }
    }
    let entry = models.providers.entry(pid.clone()).or_default();
    entry.name = req.req_provider_name.clone();
    entry.api = req.req_provider_api.clone();
    entry.base_url = req.req_provider_base_url.clone();
    entry.models = req
        .req_models
        .iter()
        .map(|m| wb_domain::provider::ModelCfg {
            id: m.model_id.clone(),
            name: m.model_name.clone(),
            reasoning: m.model_reasoning,
            vision: m.model_vision,
            context_window: m.model_context_window,
            max_tokens: m.model_max_tokens,
        })
        .collect();
    sv.providers.save_models(&models)?;
    if let Some(k) = &req.req_provider_api_key {
        if !k.is_empty() {
            let mut auth = sv.providers.load_auth()?;
            auth.providers.insert(
                pid.clone(),
                wb_domain::provider::AuthEntry::ApiKey { key: k.clone() },
            );
            sv.providers.save_auth(&auth)?;
        }
    }
    Ok(pid)
}

/// 删除：默认模型指向它时连带清理。
pub async fn delete(sv: &Arc<Services>, pid: &str) -> Result<()> {
    let mut models = sv.providers.load_models()?;
    if models.providers.remove(pid).is_none() {
        return Err(AppError::new(
            code::NOT_FOUND,
            format!("没有叫 {pid} 的服务商"),
        ));
    }
    sv.providers.save_models(&models)?;
    let mut auth = sv.providers.load_auth()?;
    auth.providers.remove(pid);
    sv.providers.save_auth(&auth)?;
    let dp = sv.repo.settings_get("default_provider")?;
    if dp.as_deref() == Some(pid) {
        sv.repo.settings_set("default_provider", "")?;
        sv.repo.settings_set("default_model", "")?;
    }
    Ok(())
}

/// 连通测试：GET {base_url}/models 带鉴权头；任何 HTTP 应答 = 可达。
pub async fn test(sv: &Arc<Services>, pid: &str) -> Result<TestProviderRESP> {
    let models = sv.providers.load_models()?;
    let cfg = models
        .providers
        .get(pid)
        .ok_or_else(|| AppError::new(code::NOT_FOUND, format!("没有叫 {pid} 的服务商")))?;
    let model = cfg.models.first().map(|m| m.id.clone()).unwrap_or_default();
    let key = effective_key(sv, pid, cfg)?;
    if cfg.base_url.is_empty() {
        return Err(AppError::new(code::BASE_URL_INVALID, "接口地址为空"));
    }
    if key.is_empty() {
        return Err(AppError::new(code::KEY_MISSING, "还没有配置 API key"));
    }
    let url = format!("{}/models", cfg.base_url.trim_end_matches('/'));
    let resp = reqwest::Client::builder()
        .timeout(HTTP_TIMEOUT)
        .build()
        .map_err(|e| AppError::wrap(code::TEST_FAILED, "HTTP 客户端构建失败", e))?
        .get(&url)
        .header("Authorization", format!("Bearer {key}"))
        .header("x-api-key", &key)
        .send()
        .await;
    match resp {
        Ok(r) => {
            let status = r.status().as_u16();
            Ok(match status {
                200..=299 => TestProviderRESP {
                    resp_ok: true,
                    resp_model: model,
                    resp_message: format!("连通正常（HTTP {status}）"),
                },
                401 | 403 => TestProviderRESP {
                    resp_ok: false,
                    resp_model: model,
                    resp_message: "服务可达但密钥被拒绝（401/403）".into(),
                },
                _ => TestProviderRESP {
                    resp_ok: true,
                    resp_model: model,
                    resp_message: format!("服务可达（HTTP {status}，模型清单路径可能不同）"),
                },
            })
        }
        Err(e) => Ok(TestProviderRESP {
            resp_ok: false,
            resp_model: model,
            resp_message: format!("无法连接：{e}"),
        }),
    }
}

/// 明文查看密钥。
pub async fn reveal(sv: &Arc<Services>, pid: &str) -> Result<RevealProviderRESP> {
    let models = sv.providers.load_models()?;
    let cfg = models
        .providers
        .get(pid)
        .ok_or_else(|| AppError::new(code::NOT_FOUND, format!("没有叫 {pid} 的服务商")))?;
    let key = effective_key(sv, pid, cfg)?;
    if key.is_empty() {
        return Err(AppError::new(code::KEY_MISSING, "该服务商还没有配置密钥"));
    }
    Ok(RevealProviderRESP {
        resp_provider_id: pid.to_string(),
        resp_api_key: key,
    })
}

/// 设默认模型。
pub async fn set_default(sv: &Arc<Services>, pid: &str, mid: &str) -> Result<()> {
    let models = sv.providers.load_models()?;
    let cfg = models
        .providers
        .get(pid)
        .ok_or_else(|| AppError::new(code::NOT_FOUND, format!("没有叫 {pid} 的服务商")))?;
    if !cfg.models.iter().any(|m| m.id == mid) {
        return Err(AppError::new(
            code::MODEL_NOT_FOUND,
            format!("{pid} 下没有模型 {mid}"),
        ));
    }
    sv.repo.settings_set("default_provider", pid)?;
    sv.repo.settings_set("default_model", mid)?;
    Ok(())
}

/// 从上游拉模型清单（OpenAI / Anthropic 两种形态都试）。
pub async fn fetch_models(
    sv: &Arc<Services>,
    pid: Option<&str>,
    _api: Option<&str>,
    base_url: Option<&str>,
    api_key: Option<&str>,
) -> Result<Vec<String>> {
    let (url, key) = match pid {
        Some(p) => {
            let models = sv.providers.load_models()?;
            let cfg = models
                .providers
                .get(p)
                .ok_or_else(|| AppError::new(code::NOT_FOUND, format!("没有叫 {p} 的服务商")))?;
            (cfg.base_url.clone(), effective_key(sv, p, cfg)?)
        }
        None => {
            let u = base_url.unwrap_or("").trim_end_matches('/').to_string();
            if u.is_empty() {
                return Err(AppError::new(code::BASE_URL_INVALID, "base_url 为空"));
            }
            (u, api_key.unwrap_or("").to_string())
        }
    };
    let client = reqwest::Client::builder()
        .timeout(HTTP_TIMEOUT)
        .build()
        .map_err(|e| AppError::wrap(code::TEST_FAILED, "HTTP 客户端构建失败", e))?;
    let resp = client
        .get(format!("{url}/models"))
        .header("Authorization", format!("Bearer {key}"))
        .header("x-api-key", &key)
        .header("anthropic-version", "2023-06-01")
        .send()
        .await
        .map_err(|e| AppError::new(code::TEST_FAILED, format!("拉取失败：{e}")))?;
    let status = resp.status();
    let text = resp.text().await.unwrap_or_default();
    if !status.is_success() {
        return Err(AppError::new(
            code::TEST_FAILED,
            format!("上游返回 HTTP {}", status.as_u16()),
        ));
    }
    let v: serde_json::Value = serde_json::from_str(&text)
        .map_err(|_| AppError::new(code::VALIDATE_FAILED, "上游返回的不是 JSON"))?;
    let ids: Vec<String> = v
        .get("data")
        .and_then(|d| d.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|m| m.get("id").and_then(|i| i.as_str()).map(|s| s.to_string()))
                .collect()
        })
        .unwrap_or_default();
    Ok(ids)
}

/// 可选模型平铺（跨 provider）。
pub async fn available(sv: &Arc<Services>) -> Result<Vec<AvailableModelVO>> {
    let models = sv.providers.load_models()?;
    let mut out = Vec::new();
    for (pid, cfg) in &models.providers {
        for m in &cfg.models {
            out.push(AvailableModelVO {
                model_id: m.id.clone(),
                model_name: m.name.clone(),
                model_provider: pid.clone(),
                model_context_window: m.context_window,
                model_max_tokens: m.max_tokens,
                model_reasoning: m.reasoning,
                model_vision: m.vision,
            });
        }
    }
    Ok(out)
}
