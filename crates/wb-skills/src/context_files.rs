//! 项目上下文文件：从工作区往上找 AGENTS.md 一类的指令文件，注入系统提示词。
//!
//! 模型不会主动去读 `AGENTS.md`——不注入就等于这份约定不存在。发现规则刻意做成
//! 「最近的 wins + 祖先合并」：离当前目录最近的那份放最后（模型对末尾更敏感）。

use std::path::{Path, PathBuf};

use wb_pkg::fsutil;

/// 候选文件名，按优先级排列（同名目录内先命中先取）。
const CANDIDATES: [&str; 5] = [
    "AGENTS.override.md",
    "AGENTS.md",
    "AGENTS.MD",
    "CLAUDE.md",
    "CLAUDE.MD",
];

/// 单文件注入上限：一份几万字的规范会把上下文窗口吃掉，截断并说明。
const MAX_FILE_BYTES: usize = 32 * 1024;

/// 向上回溯的层数上限：再往上就是磁盘根，那里不会有项目约定。
const MAX_ANCESTOR_DEPTH: usize = 8;

/// 一份被发现的上下文文件。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ContextFile {
    /// 绝对路径（提示词里原样报给模型，便于它自己再去细读）
    pub path: PathBuf,
    /// 文件正文（已按上限截断）
    pub content: String,
}

/// 在 `workspace` 及其祖先目录里发现上下文文件；由远到近返回（近的在后）。
/// 同一目录只取一个候选文件：`AGENTS.override.md` 存在时它就是这一层的唯一真相。
pub fn discover(workspace: &Path) -> Vec<ContextFile> {
    let mut found: Vec<ContextFile> = Vec::new();
    let mut dir = workspace.to_path_buf();
    for _ in 0..=MAX_ANCESTOR_DEPTH {
        if let Some(file) = read_one(&dir) {
            found.push(file);
        }
        match dir.parent() {
            Some(parent) if parent != dir => dir = parent.to_path_buf(),
            _ => break,
        }
    }
    // 现在是由近到远，反过来让最近的排在最后
    found.reverse();
    found
}

/// 读一个目录里的第一个候选文件；没有或读不出内容返回 None。
fn read_one(dir: &Path) -> Option<ContextFile> {
    for name in CANDIDATES {
        let path = dir.join(name);
        if !path.is_file() {
            continue;
        }
        let Ok(text) = fsutil::read_to_string(&path) else {
            continue;
        };
        if text.trim().is_empty() {
            // 空文件不贡献内容，但也不继续找同目录下一个：这是作者的有意留白
            return None;
        }
        return Some(ContextFile {
            path,
            content: clip(text),
        });
    }
    None
}

/// 按字节上限截断：只在字符边界切，截断时补一行说明。
fn clip(text: String) -> String {
    if text.len() <= MAX_FILE_BYTES {
        return text;
    }
    let mut end = MAX_FILE_BYTES;
    while end > 0 && !text.is_char_boundary(end) {
        end -= 1;
    }
    let mut out = text[..end].to_string();
    out.push_str("\n\n[此文件超长，以上为前 32KB；需要更多内容请用 read 工具按行读取]");
    out
}

/// 渲染成系统提示词里的 `<project_instructions>` 段；没有文件时返回空串。
pub fn render(files: &[ContextFile]) -> String {
    if files.is_empty() {
        return String::new();
    }
    let mut out = String::from("Project-specific instructions and guidelines:\n");
    for f in files {
        let path = f.path.display().to_string().replace('\\', "/");
        out.push_str(&format!(
            "\n<project_instructions path=\"{path}\">\n{}\n</project_instructions>\n",
            f.content.trim()
        ));
    }
    out
}
