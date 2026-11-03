//! 一轮运行的模型解析：思考档位归一化 + 默认 provider/model 选取。
//! 与发送流程拆开：这里只做「用哪个模型、用多大窗口」的决定，不碰会话与事件。

use std::sync::Arc;

use wb_domain::error::session as code;
use wb_pkg::{AppError, Result};

use super::Services;

/// 思考档位：模型不支持推理时一律关掉（送了也没用，只会让上游报参数错）。
/// 未知档位码回落到 medium（界面与后端版本错位时不让整轮失败），已认的档位原样透传。
pub(crate) fn thinking_level(
    session_level: Option<&str>,
    supports_reasoning: bool,
) -> wb_provider::ThinkingLevel {
    if !supports_reasoning {
        return wb_provider::ThinkingLevel::Off;
    }
    session_level
        .and_then(wb_provider::ThinkingLevel::parse)
        .unwrap_or(wb_provider::ThinkingLevel::Medium)
}

/// 解析默认模型：设置键 → 第一个有 key 的 provider。
pub(crate) fn resolve_default(
    sv: &Arc<Services>,
) -> Result<(String, String, i64, wb_provider::config::ResolvedProvider)> {
    let dp = sv.repo.settings_get("default_provider")?;
    let dm = sv.repo.settings_get("default_model")?;
    let models = sv.providers.load_models()?;
    let (pid, mid) = match (dp, dm) {
        (Some(p), Some(m)) if models.providers.contains_key(&p) => (p, m),
        _ => {
            let (p, cfg) = models
                .providers
                .iter()
                .find(|(_, c)| !c.api_key.is_empty() || !c.models.is_empty())
                .ok_or_else(|| {
                    AppError::new(
                        code::NOT_READY,
                        "还没有可用的模型配置，先去设置页添加服务商",
                    )
                })?;
            let m = cfg.models.first().map(|m| m.id.clone()).unwrap_or_default();
            (p.clone(), m)
        }
    };
    let resolved = sv.providers.resolve(&pid, Some(&mid))?;
    let window = resolved.context_window;
    Ok((pid, mid, window, resolved))
}
