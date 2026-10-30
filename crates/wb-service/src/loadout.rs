//! 装载配置（loadout）：工具 / 技能开关的命名预设。full 与 minimal 内置，其余存 settings KV。

use std::sync::Arc;

use wb_domain::feature::{LoadoutListRESP, LoadoutPreset, LoadoutREQ};
use wb_pkg::{AppError, Result};

use super::features::refresh_registry;
use super::Services;

/// 用户预设的 KV 键。
const KV_KEY: &str = "loadout_presets";

/// 内置最小集：只保留浏览与编辑类工具。
const MINIMAL_TOOLS: [&str; 6] = ["read", "write", "edit", "grep", "find", "ls"];

/// 读用户预设（KV 损坏按空处理）。
fn user_presets(sv: &Services) -> Vec<LoadoutPreset> {
    sv.repo
        .settings_get(KV_KEY)
        .ok()
        .flatten()
        .and_then(|raw| serde_json::from_str(&raw).ok())
        .unwrap_or_default()
}

/// 用户预设写回。
fn save_user_presets(sv: &Services, presets: &[LoadoutPreset]) -> Result<()> {
    let text = serde_json::to_string(presets)
        .map_err(|e| AppError::new("2002", format!("预设序列化失败：{e}")))?;
    sv.repo.settings_set(KV_KEY, &text)
}

/// 预设清单：内置在前。
pub fn list(sv: &Arc<Services>) -> LoadoutListRESP {
    let mut out = vec![
        LoadoutPreset {
            loadout_name: "full".into(),
            loadout_tools: sv
                .registry
                .read()
                .unwrap()
                .all()
                .iter()
                .filter(|t| t.name().starts_with(|c: char| c.is_ascii_lowercase()))
                .map(|t| t.name().to_string())
                .collect(),
            loadout_skills: sv
                .skills
                .read()
                .unwrap()
                .list()
                .iter()
                .map(|s| s.name.clone())
                .collect(),
        },
        LoadoutPreset {
            loadout_name: "minimal".into(),
            loadout_tools: MINIMAL_TOOLS.iter().map(|s| s.to_string()).collect(),
            loadout_skills: Vec::new(),
        },
    ];
    out.extend(user_presets(sv));
    LoadoutListRESP { resp_presets: out }
}

/// 把当前启用的工具 / 技能集合存为用户预设。
pub async fn save(sv: &Arc<Services>, name: &str) -> Result<()> {
    let name = name.trim();
    if name.is_empty() || name == "full" || name == "minimal" {
        return Err(AppError::new(
            "1001",
            "预设名不能为空，且不能叫 full / minimal",
        ));
    }
    let tools: Vec<String> = sv
        .registry
        .read()
        .unwrap()
        .all()
        .iter()
        .filter(|t| sv.repo.tool_enabled(t.name()).unwrap_or(true))
        .map(|t| t.name().to_string())
        .collect();
    let skills: Vec<String> = sv
        .skills
        .read()
        .unwrap()
        .list()
        .iter()
        .filter(|s| sv.repo.skill_enabled(&s.name).unwrap_or(true))
        .map(|s| s.name.clone())
        .collect();
    let mut presets = user_presets(sv);
    presets.retain(|p| p.loadout_name != name);
    presets.push(LoadoutPreset {
        loadout_name: name.to_string(),
        loadout_tools: tools,
        loadout_skills: skills,
    });
    save_user_presets(sv, &presets)
}

/// 应用预设：先全停再按集合启用，然后重建注册表。
pub async fn apply(sv: &Arc<Services>, name: &str) -> Result<()> {
    let tools: Vec<String>;
    let skills: Vec<String>;
    match name {
        "full" => {
            tools = sv
                .registry
                .read()
                .unwrap()
                .all()
                .iter()
                .map(|t| t.name().to_string())
                .collect();
            skills = sv
                .skills
                .read()
                .unwrap()
                .list()
                .iter()
                .map(|s| s.name.clone())
                .collect();
        }
        "minimal" => {
            tools = MINIMAL_TOOLS.iter().map(|s| s.to_string()).collect();
            skills = Vec::new();
        }
        other => {
            let preset = user_presets(sv)
                .into_iter()
                .find(|p| p.loadout_name == other)
                .ok_or_else(|| AppError::new("1004", format!("没有叫 {other} 的预设")))?;
            tools = preset.loadout_tools;
            skills = preset.loadout_skills;
        }
    }
    // 全部工具：在集合内启用，集合外停用（MCP 工具同样受管）
    let all_tools: Vec<String> = sv
        .registry
        .read()
        .unwrap()
        .all()
        .iter()
        .map(|t| t.name().to_string())
        .collect();
    for t in &all_tools {
        sv.repo.tool_set_enabled(t, tools.contains(t))?;
    }
    let all_skills: Vec<String> = sv
        .skills
        .read()
        .unwrap()
        .list()
        .iter()
        .map(|s| s.name.clone())
        .collect();
    for s in &all_skills {
        sv.repo.skill_set_enabled(s, skills.contains(s))?;
    }
    refresh_registry(sv);
    Ok(())
}

/// 删除用户预设。
pub async fn delete(sv: &Arc<Services>, req: &LoadoutREQ) -> Result<()> {
    let mut presets = user_presets(sv);
    let before = presets.len();
    presets.retain(|p| p.loadout_name != req.loadout_name);
    if presets.len() == before {
        return Err(AppError::new(
            "1004",
            format!("没有叫 {} 的预设", req.loadout_name),
        ));
    }
    save_user_presets(sv, &presets)
}
