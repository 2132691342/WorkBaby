//! 资源编排：技能管理（建/导/删/正文）与工具视图。

use std::path::{Path, PathBuf};
use std::sync::Arc;

use wb_domain::error::{common, skill as skcode};
use wb_domain::skill::{SkillContentRESP, SkillImportRESP};
use wb_pkg::{AppError, Result};

use super::Services;

/// 内置帮助文档（编译期打进二进制）。
pub static DOCS: include_dir::Dir =
    include_dir::include_dir!("$CARGO_MANIFEST_DIR/../../assets/docs");

// ---- 技能管理 ----

/// 用户技能目录。
fn user_skills_dir(sv: &Services) -> PathBuf {
    sv.data_dir.join("skills").join("user")
}

/// 三区重扫（技能文件落盘后调用）。
fn rescan(sv: &Arc<Services>) -> Result<()> {
    let ws = sv.workspace.lock().unwrap().clone();
    let registry = wb_skills::SkillRegistry::scan(
        &sv.data_dir.join("skills").join("builtin"),
        &user_skills_dir(sv),
        &PathBuf::from(&ws).join(".workbaby").join("skills"),
    );
    *sv.skills.write().unwrap() = registry;
    Ok(())
}

/// 新建技能：写用户区 SKILL.md。
pub async fn skill_create(
    sv: &Arc<Services>,
    name: &str,
    description: &str,
    body: &str,
) -> Result<()> {
    if !wb_skills::frontmatter::valid_name(name) {
        return Err(AppError::new(
            skcode::NAME_INVALID,
            format!("技能名非法：{name}"),
        ));
    }
    if !wb_skills::frontmatter::valid_description(description) {
        return Err(AppError::new(
            skcode::DESC_INVALID,
            "description 缺失或超长",
        ));
    }
    let dir = user_skills_dir(sv).join(name);
    if dir.exists() {
        return Err(AppError::new(
            skcode::NAME_DUP,
            format!("已存在同名技能 {name}"),
        ));
    }
    std::fs::create_dir_all(&dir)
        .map_err(|e| AppError::wrap(common::IO_FAILED, "建技能目录失败", e))?;
    let md = format!("---\nname: {name}\ndescription: {description}\n---\n\n{body}\n");
    std::fs::write(dir.join("SKILL.md"), md)
        .map_err(|e| AppError::wrap(common::IO_FAILED, "写 SKILL.md 失败", e))?;
    rescan(sv)
}

/// 从目录导入：源目录（含 SKILL.md）拷入用户区。
pub async fn skill_import(sv: &Arc<Services>, paths: &[String]) -> Result<SkillImportRESP> {
    let mut resp = SkillImportRESP::default();
    let user = user_skills_dir(sv);
    for p in paths {
        let src = PathBuf::from(p);
        let md = src.join("SKILL.md");
        if !md.is_file() {
            resp.resp_skipped
                .push(format!("{}（没有 SKILL.md）", src.display()));
            continue;
        }
        let name = src
            .file_name()
            .map(|n| n.to_string_lossy().into_owned())
            .unwrap_or_default();
        if name.is_empty() || !wb_skills::frontmatter::valid_name(&name) {
            resp.resp_skipped
                .push(format!("{}（目录名非法）", src.display()));
            continue;
        }
        let dst = user.join(&name);
        if dst.exists() {
            resp.resp_skipped.push(format!("{name}（已存在）"));
            continue;
        }
        match copy_dir(&src, &dst) {
            Ok(()) => resp.resp_imported += 1,
            Err(e) => resp.resp_skipped.push(format!("{name}（{}）", e.message)),
        }
    }
    rescan(sv)?;
    Ok(resp)
}

/// zip 导入：base64 解码后解压；zip 顶层单目录时剥掉。
pub async fn skill_import_zip(sv: &Arc<Services>, data_b64: &str) -> Result<SkillImportRESP> {
    use base64::Engine;
    let raw = base64::engine::general_purpose::STANDARD
        .decode(data_b64.trim())
        .map_err(|e| AppError::new(common::ARCHIVE_INVALID, format!("base64 解码失败：{e}")))?;
    let reader = std::io::Cursor::new(raw);
    let mut archive = zip::ZipArchive::new(reader)
        .map_err(|e| AppError::new(common::ARCHIVE_INVALID, format!("zip 解析失败：{e}")))?;
    let mut resp = SkillImportRESP::default();
    let user = user_skills_dir(sv);
    // 顶层目录名探测：全部条目共享第一段时视为包根
    let mut top: Option<String> = None;
    for i in 0..archive.len() {
        let f = archive
            .by_index_raw(i)
            .map_err(|e| AppError::new(common::ARCHIVE_INVALID, e.to_string()))?;
        let name = f.name().map(|n| n.to_string()).unwrap_or_default();
        let first = name.split('/').next().unwrap_or("").to_string();
        match &top {
            None => top = Some(first),
            Some(t) => {
                if *t != first {
                    top = Some(String::new());
                    break;
                }
            }
        }
    }
    let top = top.unwrap_or_default();
    let mut seen_dirs = std::collections::BTreeSet::new();
    for i in 0..archive.len() {
        let mut f = archive
            .by_index(i)
            .map_err(|e| AppError::new(common::ARCHIVE_INVALID, e.to_string()))?;
        let rel = f.name().map(|n| n.to_string()).unwrap_or_default();
        if rel.contains("..") {
            continue;
        }
        let rel = if !top.is_empty() {
            match rel.strip_prefix(&format!("{top}/")) {
                Some(r) => r.to_string(),
                None => continue,
            }
        } else {
            rel.clone()
        };
        if rel.is_empty() {
            continue;
        }
        let dst = user.join(&rel);
        if f.is_dir() {
            let _ = std::fs::create_dir_all(&dst);
            continue;
        }
        let parent = dst.parent().unwrap_or(Path::new(".")).to_path_buf();
        if seen_dirs.insert(parent.clone()) {
            let _ = std::fs::create_dir_all(&parent);
        }
        let mut bytes = Vec::new();
        std::io::Read::read_to_end(&mut f, &mut bytes)
            .map_err(|e| AppError::new(common::ARCHIVE_INVALID, e.to_string()))?;
        std::fs::write(&dst, bytes)
            .map_err(|e| AppError::wrap(common::IO_FAILED, "解压写入失败", e))?;
    }
    // 按解出来的技能目录对账：有 SKILL.md 的目录算成功
    if let Ok(entries) = std::fs::read_dir(&user) {
        for e in entries.flatten() {
            if e.path().join("SKILL.md").is_file() {
                resp.resp_imported += 1;
            }
        }
    }
    rescan(sv)?;
    Ok(resp)
}

/// 递归拷目录。
fn copy_dir(src: &Path, dst: &Path) -> Result<()> {
    std::fs::create_dir_all(dst).map_err(|e| AppError::wrap(common::IO_FAILED, "建目录失败", e))?;
    for entry in
        std::fs::read_dir(src).map_err(|e| AppError::wrap(common::IO_FAILED, "读目录失败", e))?
    {
        let entry = entry.map_err(|e| AppError::wrap(common::IO_FAILED, "读目录失败", e))?;
        let ty = entry
            .file_type()
            .map_err(|e| AppError::wrap(common::IO_FAILED, "读类型失败", e))?;
        let target = dst.join(entry.file_name());
        if ty.is_dir() {
            copy_dir(&entry.path(), &target)?;
        } else {
            std::fs::copy(entry.path(), &target)
                .map_err(|e| AppError::wrap(common::IO_FAILED, "拷贝失败", e))?;
        }
    }
    Ok(())
}

/// 删除技能：内置锁死；用户 / 工作区删目录。
pub async fn skill_delete(sv: &Arc<Services>, name: &str) -> Result<()> {
    let entry = sv
        .skills
        .read()
        .unwrap()
        .get(name)
        .cloned()
        .ok_or_else(|| AppError::new(skcode::PARSE_FAILED, format!("没有叫 {name} 的技能")))?;
    match entry.source {
        wb_domain::skill::SkillSource::Builtin => Err(AppError::new(
            skcode::BUILTIN_LOCKED,
            "内置技能不可删除，只能停用",
        )),
        wb_domain::skill::SkillSource::User => {
            let dir = user_skills_dir(sv).join(name);
            std::fs::remove_dir_all(&dir)
                .map_err(|e| AppError::wrap(common::IO_FAILED, "删除技能目录失败", e))?;
            rescan(sv)
        }
        wb_domain::skill::SkillSource::Workspace => {
            let ws = sv.workspace.lock().unwrap().clone();
            let dir = PathBuf::from(&ws)
                .join(".workbaby")
                .join("skills")
                .join(name);
            std::fs::remove_dir_all(&dir)
                .map_err(|e| AppError::wrap(common::IO_FAILED, "删除技能目录失败", e))?;
            rescan(sv)
        }
    }
}

/// 技能正文（SKILL.md 原文）。
pub fn skill_content(sv: &Arc<Services>, name: &str) -> Result<SkillContentRESP> {
    let entry = sv
        .skills
        .read()
        .unwrap()
        .get(name)
        .cloned()
        .ok_or_else(|| AppError::new(skcode::PARSE_FAILED, format!("没有叫 {name} 的技能")))?;
    let text = std::fs::read_to_string(&entry.location)
        .map_err(|e| AppError::wrap(common::IO_FAILED, "读 SKILL.md 失败", e))?;
    Ok(SkillContentRESP { resp_content: text })
}

// ---- 文件树 ----

/// 全部工具视图（含停用；inspect 与 /tools 共用）。
pub fn tools_all_vo(sv: &Arc<Services>) -> Vec<wb_domain::tool::ToolVO> {
    sv.registry
        .read()
        .unwrap()
        .all()
        .iter()
        .map(|t| wb_domain::tool::ToolVO {
            tool_name: t.name().to_string(),
            tool_label: t.label().to_string(),
            tool_description: t.description().to_string(),
            tool_source: if t.name().starts_with("mcp__") {
                "mcp".into()
            } else {
                "builtin".into()
            },
            tool_enabled: sv.repo.tool_enabled(t.name()).unwrap_or(true),
            tool_read_only: t.annotations().read_only,
        })
        .collect()
}
