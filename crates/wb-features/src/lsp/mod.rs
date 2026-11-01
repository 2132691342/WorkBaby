//! LSP 功能：语言无关的诊断与快速修复（语言服务器需用户自装）。

pub mod adapters;
pub mod client;
pub mod diagnostics;
pub mod edits;
pub mod fix;

use std::path::Path;

use adapters::ServerSpec;

/// 默认超时（毫秒）。
pub const DEFAULT_TIMEOUT_MS: u64 = 15_000;
/// 单次诊断最多检查的文件数。
pub const DEFAULT_FILE_LIMIT: usize = 50;

/// 遍历时始终跳过的目录（与语言无关）。
const GLOBAL_SKIP_DIRS: &[&str] = &[
    ".git",
    "node_modules",
    "target",
    "dist",
    "build",
    "__pycache__",
    ".venv",
    "venv",
    ".next",
    ".turbo",
    "out",
    "coverage",
];

/// 代码与诊断的错误码。
pub mod codes {
    /// 语言服务器未安装
    pub use wb_domain::error::tool::LSP_NOT_INSTALLED as NOT_INSTALLED;
    /// 没有匹配的文件类型
    pub use wb_domain::error::tool::LSP_NO_MATCH as NO_MATCH;
    /// 语言服务器返回错误响应
    pub use wb_domain::error::tool::LSP_SERVER_ERROR as SERVER_ERROR;
}

/// 收集工作区内属于某服务器扩展名的文件（按 mtime 倒序取最近）。
pub fn collect_files(spec: &ServerSpec, root: &Path, limit: usize) -> Vec<std::path::PathBuf> {
    let mut found: Vec<(std::time::SystemTime, std::path::PathBuf)> = Vec::new();
    let walker = ignore::WalkBuilder::new(root)
        .hidden(false)
        .git_ignore(true)
        .git_global(false)
        .parents(false)
        .build();
    for entry in walker.flatten() {
        if found.len() >= limit * 4 {
            break;
        }
        let path = entry.path();
        if !path.is_file() {
            continue;
        }
        let rel = path.strip_prefix(root).unwrap_or(path);
        if rel
            .components()
            .any(|c| GLOBAL_SKIP_DIRS.contains(&c.as_os_str().to_str().unwrap_or("")))
        {
            continue;
        }
        if rel.components().any(|c| {
            spec.server_skip_dirs
                .contains(&c.as_os_str().to_str().unwrap_or(""))
        }) {
            continue;
        }
        if !adapters::accepts(spec, path) {
            continue;
        }
        let mtime = entry
            .metadata()
            .ok()
            .and_then(|m| m.modified().ok())
            .unwrap_or(std::time::SystemTime::UNIX_EPOCH);
        found.push((mtime, path.to_path_buf()));
    }
    found.sort_by_key(|e| std::cmp::Reverse(e.0));
    found.truncate(limit);
    found.into_iter().map(|(_, p)| p).collect()
}

/// 探测某程序是否在 PATH 中（纯路径查找，不起子进程）。
pub fn command_exists(command: &str) -> bool {
    wb_pkg::proc::resolve_command(command).is_some()
}

/// 收集可用的语言服务器清单（界面展示用）。
pub fn installed_servers() -> Vec<(String, bool)> {
    adapters::SERVERS
        .iter()
        .map(|s| (s.server_name.to_string(), command_exists(s.server_command)))
        .collect()
}

/// 是否至少装了一个语言服务器（装配时据此决定是否把工具并进注册表）。
pub fn any_server_installed() -> bool {
    adapters::SERVERS
        .iter()
        .any(|s| command_exists(s.server_command))
}
