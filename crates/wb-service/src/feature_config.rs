//! 功能配置的 schema 构建与保存：界面按字段表渲染表单，不写死每项功能的表单。
//! 可配项只有三项（web / memory / lsp）；其余功能没有配置，`resp_configurable` 为 false。

use std::collections::BTreeMap;
use std::sync::Arc;

use serde_json::{json, Value};

use wb_domain::feature::{
    FeatureConfigFieldVO, FeatureConfigOptionVO, FeatureConfigRESP, FeatureConfigSaveREQ,
};
use wb_pkg::{AppError, Result};

use super::Services;

/// 有配置的功能码。
pub const CONFIGURABLE: [&str; 3] = ["web", "memory", "lsp"];

/// 设置 KV 的键名。
fn kv_key(feature_code: &str) -> String {
    format!("feature_config_{feature_code}")
}

/// 读某功能的配置响应；未知或有误的配置一律回落默认值。
pub fn get(sv: &Arc<Services>, feature_code: &str) -> Result<FeatureConfigRESP> {
    let raw = read_raw(sv, feature_code)?;
    let fields = match feature_code {
        "web" => web_fields(&raw),
        "memory" => memory_fields(&raw),
        "lsp" => lsp_fields(&raw),
        _ => {
            return Ok(FeatureConfigRESP {
                resp_feature_code: feature_code.to_string(),
                resp_configurable: false,
                resp_fields: Vec::new(),
            })
        }
    };
    Ok(FeatureConfigRESP {
        resp_feature_code: feature_code.to_string(),
        resp_configurable: true,
        resp_fields: fields,
    })
}

/// 保存：把新值并进现有配置再整体写回，未提交的键保持原样。
pub fn save(sv: &Arc<Services>, feature_code: &str, req: &FeatureConfigSaveREQ) -> Result<()> {
    if !CONFIGURABLE.contains(&feature_code) {
        return Err(AppError::new(
            wb_domain::error::common::PARAM_INVALID,
            format!("{feature_code} 没有可配项"),
        ));
    }
    // 以「当前配置序列化出的完整对象」为底，再把提交的键覆盖上去
    let mut base = match feature_code {
        "web" => json!(wb_features::config::WebConfig::default()),
        "memory" => json!(wb_features::config::MemoryConfig::default()),
        _ => json!(wb_features::config::LspConfig::default()),
    };
    let current = read_raw(sv, feature_code)?;
    if let Some(parsed) = current
        .as_deref()
        .and_then(|t| serde_json::from_str::<Value>(t).ok())
    {
        if let (Some(dst), Some(src)) = (base.as_object_mut(), parsed.as_object()) {
            for (k, v) in src {
                dst.insert(k.clone(), v.clone());
            }
        }
    }
    if let Some(dst) = base.as_object_mut() {
        for (k, v) in &req.config_values {
            dst.insert(k.clone(), v.clone());
        }
    }
    sv.repo
        .settings_set(&kv_key(feature_code), &base.to_string())
}

/// 读原始配置串。
fn read_raw(sv: &Arc<Services>, feature_code: &str) -> Result<Option<String>> {
    sv.repo.settings_get(&kv_key(feature_code))
}

/// 从原始串解出某段配置；坏 JSON 用默认值。
fn parse_or_default<T: Default + for<'de> serde::Deserialize<'de>>(raw: &Option<String>) -> T {
    raw.as_deref()
        .and_then(|t| serde_json::from_str(t).ok())
        .unwrap_or_default()
}

/// web 的字段表。
fn web_fields(raw: &Option<String>) -> Vec<FeatureConfigFieldVO> {
    let cfg: wb_features::config::WebConfig = parse_or_default(raw);
    let cfg = cfg.sanitized();
    // 候选源来自真实源表；缺密钥的源照样列出，但带不可用原因
    let missing = wb_features::web_providers::missing_key_codes();
    let mut options = vec![FeatureConfigOptionVO {
        option_value: String::new(),
        option_label: "自动（有密钥的优先，公开源兜底）".into(),
        option_reason: String::new(),
    }];
    options.extend(wb_features::web_providers::codes().into_iter().map(|code| {
        let label = wb_features::web_providers::by_code(code)
            .map(|p| p.provider_label.to_string())
            .unwrap_or_else(|| code.to_string());
        let reason = if missing.contains(&code) {
            "缺密钥".to_string()
        } else {
            String::new()
        };
        FeatureConfigOptionVO {
            option_value: code.to_string(),
            option_label: label,
            option_reason: reason,
        }
    }));
    vec![
        FeatureConfigFieldVO {
            field_name: "web_provider".into(),
            field_label: "搜索源".into(),
            field_description: "锁定单一搜索源；自动模式下按优先级排队".into(),
            field_kind: "select".into(),
            field_value: json!(cfg.web_provider),
            field_options: options,
        },
        FeatureConfigFieldVO {
            field_name: "web_search_limit".into(),
            field_label: "结果条数".into(),
            field_description: "单次搜索返回的结果上限（1-20）".into(),
            field_kind: "number".into(),
            field_value: json!(cfg.web_search_limit),
            field_options: Vec::new(),
        },
        FeatureConfigFieldVO {
            field_name: "web_proxy".into(),
            field_label: "HTTP 代理".into(),
            field_description: "留空直连；形如 http://127.0.0.1:7890".into(),
            field_kind: "text".into(),
            field_value: json!(cfg.web_proxy),
            field_options: Vec::new(),
        },
        FeatureConfigFieldVO {
            field_name: "web_keys".into(),
            field_label: "搜索源密钥".into(),
            field_description: "每行一条 `源码=密钥`，如 brave=xxx；也可以改用环境变量（BRAVE_API_KEY 等），此处优先。".into(),
            field_kind: "text".into(),
            field_value: json!(cfg.web_keys),
            field_options: Vec::new(),
        },
    ]
}

/// memory 的字段表。
fn memory_fields(raw: &Option<String>) -> Vec<FeatureConfigFieldVO> {
    let cfg: wb_features::config::MemoryConfig = parse_or_default(raw);
    let cfg = cfg.sanitized();
    vec![
        FeatureConfigFieldVO {
            field_name: "memory_auto_threshold".into(),
            field_label: "自动整合阈值".into(),
            field_description: "记忆条数超过它就自动合并重复项；0 = 关闭自动整合".into(),
            field_kind: "number".into(),
            field_value: json!(cfg.memory_auto_threshold),
            field_options: Vec::new(),
        },
        FeatureConfigFieldVO {
            field_name: "memory_merge_threshold".into(),
            field_label: "相似度阈值".into(),
            field_description: "达到这个相似度才判为重复（0.1-1.0），越高越保守".into(),
            field_kind: "number".into(),
            field_value: json!(cfg.memory_merge_threshold),
            field_options: Vec::new(),
        },
        FeatureConfigFieldVO {
            field_name: "memory_search_limit".into(),
            field_label: "检索条数".into(),
            field_description: "单次检索默认返回条数（1-50）".into(),
            field_kind: "number".into(),
            field_value: json!(cfg.memory_search_limit),
            field_options: Vec::new(),
        },
    ]
}

/// lsp 的字段表。
fn lsp_fields(raw: &Option<String>) -> Vec<FeatureConfigFieldVO> {
    let cfg: wb_features::config::LspConfig = parse_or_default(raw);
    let options = wb_features::lsp::adapters::SERVERS
        .iter()
        .map(|spec| {
            let reason = if wb_features::lsp::command_exists(spec.server_command) {
                String::new()
            } else {
                format!("未安装 {}", spec.server_command)
            };
            FeatureConfigOptionVO {
                option_value: spec.server_name.to_string(),
                option_label: format!(
                    "{}（{}）",
                    spec.server_name,
                    spec.server_extensions.join(" / ")
                ),
                option_reason: reason,
            }
        })
        .collect();
    vec![FeatureConfigFieldVO {
        field_name: "lsp_servers".into(),
        field_label: "启用的服务器".into(),
        field_description: "一条都不选 = 已安装的服务器全部可用".into(),
        field_kind: "multi_select".into(),
        field_value: json!(cfg.lsp_servers),
        field_options: options,
    }]
}

/// 提交值的键名白名单（防把任意键写进配置对象）。
pub fn known_fields(feature_code: &str) -> &'static [&'static str] {
    match feature_code {
        "web" => &["web_provider", "web_search_limit", "web_proxy", "web_keys"],
        "memory" => &[
            "memory_auto_threshold",
            "memory_merge_threshold",
            "memory_search_limit",
        ],
        "lsp" => &["lsp_servers"],
        _ => &[],
    }
}

/// 过滤掉不认识的键；返回被拒的键名。
pub fn reject_unknown(feature_code: &str, values: &BTreeMap<String, Value>) -> Vec<String> {
    let known = known_fields(feature_code);
    values
        .keys()
        .filter(|k| !known.contains(&k.as_str()))
        .cloned()
        .collect()
}
