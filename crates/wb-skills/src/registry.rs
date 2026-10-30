//! 技能注册表：三区扫描（内置包 / 用户 / 工作区）、同名先到先得、开关与注入文本。

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use wb_domain::skill::{SkillFrontmatter, SkillSource, SkillVO};
use wb_pkg::{fsutil, Result};

/// 内置技能包：编译期打进二进制的目录（`skills/` 相对仓库根，随包分发）。
pub static BUILTIN_SKILLS: include_dir::Dir =
    include_dir::include_dir!("$CARGO_MANIFEST_DIR/../../assets/skills");

/// 技能注册表。
pub struct SkillRegistry {
    /// 技能名 → 条目（含 location 与来源）
    entries: BTreeMap<String, SkillEntry>,
}

/// 注册表条目。
#[derive(Debug, Clone)]
pub struct SkillEntry {
    /// frontmatter
    pub frontmatter: SkillFrontmatter,
    /// 生效名
    pub name: String,
    /// SKILL.md 绝对路径（内置技能为解压后路径）
    pub location: PathBuf,
    /// 来源
    pub source: SkillSource,
}

impl SkillRegistry {
    /// 空表。
    pub fn new() -> Self {
        Self {
            entries: BTreeMap::new(),
        }
    }
}

impl Default for SkillRegistry {
    /// 空表（new 的别名，满足容器默认构造惯例）。
    fn default() -> Self {
        Self::new()
    }
}

impl SkillRegistry {
    /// 全量扫描三区并装配（builtin 先解压到磁盘，供 read 工具按 location 读取）。
    pub fn scan(builtin_dir: &Path, user_dir: &Path, workspace_dir: &Path) -> Self {
        let mut reg = Self::new();
        ensure_builtin_extracted(builtin_dir);
        // 优先级：工作区 > 用户 > 内置（先插低优先级，覆盖式插入）
        reg.scan_dir(builtin_dir, SkillSource::Builtin);
        reg.scan_dir(user_dir, SkillSource::User);
        reg.scan_dir(workspace_dir, SkillSource::Workspace);
        reg
    }

    fn scan_dir(&mut self, dir: &Path, source: SkillSource) {
        if !dir.is_dir() {
            return;
        }
        for entry in fsutil::list_dir(dir) {
            let skill_md = entry.join("SKILL.md");
            if !skill_md.is_file() {
                continue;
            }
            let Ok(text) = fsutil::read_to_string(&skill_md) else {
                continue;
            };
            let (fm, _) = match crate::parse_skill_md(&text) {
                Ok(v) => v,
                // 坏文件只跳过自己，不挡整表（specs/04）
                Err(e) => {
                    tracing::warn!(path = %skill_md.display(), "技能解析失败: {e}");
                    continue;
                }
            };
            let Some(desc) = fm.description.clone() else {
                continue;
            };
            if !crate::valid_description(&desc) {
                tracing::warn!(path = %skill_md.display(), "description 缺失或超长，跳过");
                continue;
            }
            let name = fm.name.clone().unwrap_or_else(|| {
                entry
                    .file_name()
                    .unwrap_or_default()
                    .to_string_lossy()
                    .into_owned()
            });
            if !crate::valid_name(&name) {
                tracing::warn!(name = %name, "技能名非法，跳过");
                continue;
            }
            self.entries.insert(
                name.clone(),
                SkillEntry {
                    frontmatter: fm,
                    name,
                    location: skill_md,
                    source,
                },
            );
        }
    }

    /// 全部技能视图（开关由调用方合成）。
    pub fn list(&self) -> Vec<SkillEntry> {
        self.entries.values().cloned().collect()
    }

    /// 按名取条目。
    pub fn get(&self, name: &str) -> Option<&SkillEntry> {
        self.entries.get(name)
    }

    /// 合成开关后的视图对象。
    pub fn list_vo(&self, enabled_of: impl Fn(&str) -> bool) -> Vec<SkillVO> {
        self.entries
            .values()
            .map(|e| SkillVO {
                skill_name: e.name.clone(),
                skill_description: e.frontmatter.description.clone().unwrap_or_default(),
                skill_description_zh: e.frontmatter.description_zh.clone(),
                skill_description_en: e.frontmatter.description_en.clone(),
                skill_version: e.frontmatter.version.clone(),
                skill_source: e.source,
                skill_location: e.location.display().to_string(),
                skill_enabled: enabled_of(&e.name),
            })
            .collect()
    }

    /// 注入系统提示词的 `<available_skills>` 段（禁用 disable-model-invocation 与已停用项）。
    pub fn format_available_skills(&self, enabled_of: impl Fn(&str) -> bool) -> String {
        let mut out = String::from(
            "Use the read tool to load a skill's file when the task matches its description.\n\n<available_skills>\n",
        );
        for e in self.entries.values() {
            if e.frontmatter.disable_model_invocation.unwrap_or(false) {
                continue;
            }
            if !enabled_of(&e.name) {
                continue;
            }
            out.push_str(&format!(
                "<skill><name>{}</name><description>{}</description><location>{}</location></skill>\n",
                e.name,
                e.frontmatter.description.as_deref().unwrap_or(""),
                e.location.display()
            ));
        }
        out.push_str("</available_skills>");
        out
    }
}

/// 内置包解压：带版本哨兵（包内任一文件变化即重解压，简化为目录缺失即解压 + 总字节数校验）。
fn ensure_builtin_extracted(target: &Path) {
    if target.is_dir() && fsutil::list_dir(target).len() >= 5 {
        return; // 已解压过（首次发布即含 22 个技能目录）
    }
    if extract_dir(&BUILTIN_SKILLS, target).is_err() {
        tracing::warn!("内置技能解压失败");
    }
}

fn extract_dir(dir: &include_dir::Dir, target: &Path) -> Result<()> {
    fsutil::ensure_dir(target)?;
    for f in dir.files() {
        let rel = f.path();
        let dest = target.join(rel);
        if let Some(parent) = dest.parent() {
            fsutil::ensure_dir(parent)?;
        }
        fsutil::write_atomic(&dest, f.contents())?;
    }
    for sub in dir.dirs() {
        extract_dir(sub, target)?;
    }
    Ok(())
}
