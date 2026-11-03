//! 内置工具：read / write / edit / bash / powershell / python / grep / find / ls。

pub mod bash;
pub mod edit;
pub mod find;
pub mod grep;
pub mod list;
pub mod powershell;
pub mod python;
pub mod read;
pub mod write;

use std::path::{Path, PathBuf};
use std::sync::Arc;

use wb_pkg::{AppError, Result};

use crate::ToolRegistry;

/// 把全部内置工具装进注册表。
pub fn install(registry: &mut ToolRegistry) {
    registry.register(Arc::new(read::ReadTool));
    registry.register(Arc::new(write::WriteTool));
    registry.register(Arc::new(edit::EditTool));
    registry.register(Arc::new(bash::BashTool));
    registry.register(Arc::new(powershell::PowerShellTool));
    registry.register(Arc::new(python::PythonTool));
    registry.register(Arc::new(grep::GrepTool));
    registry.register(Arc::new(find::FindTool));
    registry.register(Arc::new(list::LsTool));
}

/// 解析工作区内路径：绝对路径判越界，相对路径挂工作区；返回归一后的绝对路径。
pub fn resolve_in_workdir(workdir: &Path, raw: &str) -> Result<PathBuf> {
    let raw = raw.trim();
    if raw.is_empty() {
        return Err(AppError::new("1001", "路径不能为空"));
    }
    let p = Path::new(raw);
    if p.is_absolute() {
        wb_pkg::realpath::relative_to(workdir, p)?; // 越界即 1003
        wb_pkg::realpath::real_path(p)
    } else {
        wb_pkg::realpath::real_path(&workdir.join(p))
    }
}

/// 改动文件的展示路径：能相对工作区就相对（界面按工作区拼回绝对路径），
/// 否则保留绝对路径——两者都能被界面认出来。
pub fn rel_display(workdir: &Path, path: &Path) -> String {
    path.strip_prefix(workdir)
        .map(|p| p.display().to_string().replace('\\', "/"))
        .unwrap_or_else(|_| path.display().to_string().replace('\\', "/"))
}
