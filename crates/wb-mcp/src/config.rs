//! MCP 配置：全局 mcp.json + 工作区覆盖，同一份结构读写。

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use wb_domain::error::mcp as code;
use wb_domain::mcp::{self, McpServerConfig};
use wb_pkg::{AppError, Result};

/// 一个 server 在合并视图里的归属。
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Scope {
    /// 数据目录 mcp.json
    Global,
    /// 工作区 .workbaby/mcp.json
    Workspace,
}

impl Scope {
    /// 字符串形态（前端 scope 字段）。
    pub fn as_str(self) -> &'static str {
        match self {
            Scope::Global => "global",
            Scope::Workspace => "workspace",
        }
    }
}

/// 合并后的配置视图：同名 server 工作区覆盖全局。
pub struct McpConfig {
    /// 全局条目
    pub global: BTreeMap<String, McpServerConfig>,
    /// 工作区条目
    pub workspace: BTreeMap<String, McpServerConfig>,
    /// 全局配置文件路径
    pub global_path: PathBuf,
    /// 工作区配置文件路径
    pub workspace_path: PathBuf,
}

impl McpConfig {
    /// 装载两份配置（文件缺失视为空；解析失败报 2001 而不是吞掉）。
    pub fn load(data_dir: &Path, workspace: &Path) -> Self {
        let global_path = data_dir.join("mcp.json");
        let workspace_path = workspace.join(".workbaby").join("mcp.json");
        Self {
            global: read_map(&global_path),
            workspace: read_map(&workspace_path),
            global_path,
            workspace_path,
        }
    }

    /// 合并视图：工作区覆盖全局，附各自 scope。
    pub fn merged(&self) -> Vec<(String, McpServerConfig, Scope)> {
        let mut out = Vec::new();
        for (name, cfg) in &self.global {
            out.push((name.clone(), cfg.clone(), Scope::Global));
        }
        for (name, cfg) in &self.workspace {
            match out.iter_mut().find(|(n, _, _)| n == name) {
                Some((_, c, s)) => {
                    *c = cfg.clone();
                    *s = Scope::Workspace;
                }
                None => out.push((name.clone(), cfg.clone(), Scope::Workspace)),
            }
        }
        out
    }

    /// 单文件原文（不存在给 "{}"）。
    pub fn raw(&self, scope: Scope) -> String {
        let path = match scope {
            Scope::Global => &self.global_path,
            Scope::Workspace => &self.workspace_path,
        };
        std::fs::read_to_string(path).unwrap_or_else(|_| "{}\n".into())
    }
}

/// 解析 mcp.json：兼容 `{ "mcpServers": {...} }` 包裹与平铺两种形态。
fn read_map(path: &Path) -> BTreeMap<String, McpServerConfig> {
    let text = match std::fs::read_to_string(path) {
        Ok(t) => t,
        Err(_) => return BTreeMap::new(),
    };
    let value: serde_json::Value = match serde_json::from_str(&text) {
        Ok(v) => v,
        Err(e) => {
            tracing::warn!("mcp 配置解析失败 {}: {}", path.display(), e);
            return BTreeMap::new();
        }
    };
    let map = if let Some(inner) = value.get("mcpServers") {
        inner.clone()
    } else {
        value
    };
    serde_json::from_value::<BTreeMap<String, McpServerConfig>>(map).unwrap_or_default()
}

/// 保存一份配置原文：先按目标形态整体校验，再原子落盘（自动建目录）。
pub fn save_raw(path: &Path, raw: &str) -> Result<()> {
    let value: serde_json::Value = serde_json::from_str(raw)
        .map_err(|e| AppError::new(code::CONFIG_INVALID, format!("JSON 解析失败：{e}")))?;
    let map_value = if let Some(inner) = value.get("mcpServers") {
        inner.clone()
    } else {
        value
    };
    let map: BTreeMap<String, McpServerConfig> = serde_json::from_value(map_value)
        .map_err(|e| AppError::new(code::CONFIG_INVALID, format!("server 配置不合法：{e}")))?;
    for (name, cfg) in &map {
        mcp::validate(name, cfg)?;
    }
    wb_pkg::fsutil::ensure_dir(path.parent().unwrap_or(Path::new(".")))?;
    wb_pkg::fsutil::write_atomic(path, format!("{raw}\n").as_bytes())
}

/// 从指定 scope 删除一个 server 并落盘。
pub fn remove_server(path: &Path, name: &str) -> Result<()> {
    let text = std::fs::read_to_string(path).unwrap_or_else(|_| "{}".into());
    let mut value: serde_json::Value = serde_json::from_str(&text)
        .map_err(|e| AppError::new(code::CONFIG_INVALID, format!("JSON 解析失败：{e}")))?;
    let map = if value.get("mcpServers").is_some() {
        value.get_mut("mcpServers").unwrap()
    } else {
        &mut value
    };
    if let Some(obj) = map.as_object_mut() {
        if obj.remove(name).is_none() {
            return Err(AppError::new(
                code::CONFIG_INVALID,
                format!("没有叫 {name} 的 server"),
            ));
        }
    }
    wb_pkg::fsutil::write_atomic(
        path,
        format!(
            "{}\n",
            serde_json::to_string_pretty(&value)
                .map_err(|e| AppError::new(code::CONFIG_INVALID, format!("序列化失败：{e}")))?
        )
        .as_bytes(),
    )
}
