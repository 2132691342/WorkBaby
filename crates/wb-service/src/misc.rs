//! 资源编排（续）：内置文档、斜杠命令、工作区历史、背景图。

use std::path::PathBuf;
use std::sync::Arc;

use wb_domain::error::common;
use wb_domain::resource::{BackgroundVO, CommandVO, HelpDocRESP, HelpDocVO};
use wb_pkg::{AppError, Result};

use super::resources::DOCS;
use super::Services;

/// 内置文档清单。
pub fn docs_list() -> Vec<HelpDocVO> {
    let mut out = Vec::new();
    for f in DOCS.files() {
        let name = f
            .path()
            .file_stem()
            .map(|s| s.to_string_lossy().into_owned())
            .unwrap_or_default();
        let text = String::from_utf8_lossy(f.contents());
        let title = text
            .lines()
            .find(|l| l.starts_with("# "))
            .map(|l| l[2..].trim().to_string())
            .unwrap_or_else(|| name.clone());
        out.push(HelpDocVO {
            doc_name: name,
            doc_title: title,
        });
    }
    out.sort_by(|a, b| a.doc_name.cmp(&b.doc_name));
    out
}

/// 单篇文档。
pub fn doc_content(name: &str) -> Result<HelpDocRESP> {
    let safe = name.replace(['/', '\\', '.', ':'], "");
    let f = DOCS
        .get_file(format!("{safe}.md"))
        .ok_or_else(|| AppError::new(common::PATH_MISSING, format!("没有文档 {name}")))?;
    Ok(HelpDocRESP {
        resp_name: safe,
        resp_content: String::from_utf8_lossy(f.contents()).into_owned(),
    })
}

/// 斜杠命令：启用技能即命令（/skill:名）。
pub fn commands_list(sv: &Arc<Services>) -> Vec<CommandVO> {
    sv.skills
        .read()
        .unwrap()
        .list_vo(|n| sv.repo.skill_enabled(n).unwrap_or(true))
        .iter()
        .map(|s| CommandVO {
            command_name: format!("skill:{}", s.skill_name),
            command_description: s.skill_description.clone(),
            command_source: "skill".into(),
        })
        .collect()
}

// ---- 背景图 ----

/// 背景图设置视图。
pub fn background_get(sv: &Arc<Services>) -> BackgroundVO {
    let file = sv
        .repo
        .settings_get("background_file")
        .ok()
        .flatten()
        .unwrap_or_default();
    let opacity = sv
        .repo
        .settings_get("background_opacity")
        .ok()
        .flatten()
        .and_then(|v| v.parse().ok())
        .unwrap_or(12);
    let blur = sv
        .repo
        .settings_get("background_blur")
        .ok()
        .flatten()
        .and_then(|v| v.parse().ok())
        .unwrap_or(0);
    BackgroundVO {
        resp_background_file: file,
        resp_background_opacity: opacity,
        resp_background_blur: blur,
    }
}

/// 保存背景图：拷进数据目录（防源文件挪走），记 KV。
pub async fn background_save(
    sv: &Arc<Services>,
    path: &str,
    opacity: i64,
    blur: i64,
) -> Result<BackgroundVO> {
    if path.is_empty() {
        sv.repo.settings_set("background_file", "")?;
        return Ok(background_get(sv));
    }
    let src = PathBuf::from(path);
    if !src.is_file() {
        return Err(AppError::new(
            common::PATH_MISSING,
            format!("图片不存在：{path}"),
        ));
    }
    let ext = src
        .extension()
        .map(|e| e.to_string_lossy().into_owned())
        .unwrap_or_else(|| "png".into());
    let file = format!("background.{ext}");
    std::fs::copy(&src, sv.data_dir.join(&file))
        .map_err(|e| AppError::wrap(common::IO_FAILED, "拷贝背景图失败", e))?;
    sv.repo.settings_set("background_file", &file)?;
    sv.repo
        .settings_set("background_opacity", &opacity.to_string())?;
    sv.repo.settings_set("background_blur", &blur.to_string())?;
    Ok(BackgroundVO {
        resp_background_file: file,
        resp_background_opacity: opacity,
        resp_background_blur: blur,
    })
}
