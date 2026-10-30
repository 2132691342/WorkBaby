//! 内置功能：FeatureCtx 注入 + 功能工具（docs/FEATURES.md）。开关与注册表装配在 service。

pub mod ask_user;
pub mod context_mode;
pub mod fff;
pub mod goal;
pub mod lsp;
pub mod memory;
pub mod memory_store;
pub mod plan;
pub mod todo;
pub mod web;
pub mod web_parse;

use std::path::{Path, PathBuf};
use std::sync::Arc;

/// SSE 出口闭包（session_id, event, data）；session_id 空串 = 全局通道。
pub type Emit = Arc<dyn Fn(&str, &str, serde_json::Value) + Send + Sync>;

/// 功能运行上下文：service 装配时注入；功能代码不依赖 service。
#[derive(Clone)]
pub struct FeatureCtx {
    /// 数据目录
    pub data_dir: PathBuf,
    /// 当前工作区
    pub workspace: PathBuf,
    /// SSE 出口
    pub emit: Emit,
}

impl FeatureCtx {
    /// 功能自有文件目录（数据目录\features\{工作区编码}）。
    pub fn feature_dir(&self) -> PathBuf {
        let encoded = self
            .workspace
            .display()
            .to_string()
            .replace([char::from(92), '/', ':'], "-");
        self.data_dir.join("features").join(encoded)
    }

    /// 工作区内路径是否合法（防穿越的词法检查）。
    pub fn ensure_inside(&self, p: &Path) -> bool {
        !p.components()
            .any(|c| matches!(c, std::path::Component::ParentDir))
    }
}
