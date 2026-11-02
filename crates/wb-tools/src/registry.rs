//! Tool 契约与注册表：内核只认 `Arc<dyn Tool>`，不感知工具来源。

use std::collections::BTreeMap;
use std::path::Path;
use std::sync::Arc;

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::Result;

pub use wb_domain::events::DiffLine;

/// 一次工具调用。
#[derive(Debug, Clone)]
pub struct ToolCall {
    /// 调用 id（TC_ 前缀）
    pub call_id: String,
    /// 工具名
    pub name: String,
    /// 模型给的角色
    pub origin: String,
    /// 参数对象（未校验）
    pub arguments: Value,
}

/// 工具执行输出。
#[derive(Debug, Clone)]
pub struct ToolOutput {
    /// 结果文本（进模型上下文；已截断）
    pub text: String,
    /// 是否按错误处理
    pub is_error: bool,
    /// 耗时（毫秒）
    pub duration_ms: i64,
    /// 改动文件（相对工作区）；不产生文件改动的工具为 None
    pub file_path: Option<String>,
    /// 改动差异行；与 file_path 成对出现
    pub diff: Option<Vec<DiffLine>>,
}

impl ToolOutput {
    /// 构造一个无文件改动的输出（大多数工具的形态）。
    pub fn plain(text: impl Into<String>, is_error: bool, duration_ms: i64) -> Self {
        Self {
            text: text.into(),
            is_error,
            duration_ms,
            file_path: None,
            diff: None,
        }
    }

    /// 带上改动文件与差异行：界面在工具卡里就地着色。
    pub fn with_changes(mut self, file_path: impl Into<String>, diff: Vec<DiffLine>) -> Self {
        self.file_path = Some(file_path.into());
        self.diff = Some(diff);
        self
    }
}

/// 工具注解（给界面的行为提示）。
#[derive(Debug, Clone, Copy, Default)]
pub struct ToolAnnotations {
    /// 只读（不产生副作用）
    pub read_only: bool,
    /// 有破坏性（删除 / 覆盖）
    pub destructive: bool,
}

/// 工具契约：schema 自描述，执行异步可取消。
#[async_trait]
pub trait Tool: Send + Sync {
    /// 工具名（身份，小写+下划线）
    fn name(&self) -> &str;
    /// 显示名
    fn label(&self) -> &str;
    /// 给模型的能力说明
    fn description(&self) -> &str;
    /// 参数 JSON Schema
    fn parameters(&self) -> Value;
    /// 行为注解
    fn annotations(&self) -> ToolAnnotations {
        ToolAnnotations::default()
    }

    /// 执行：args 已过 schema 校验；cancel 触发应尽快返回 7002。
    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        workdir: &Path,
    ) -> Result<ToolOutput>;
}

/// 注册表：名字 → 工具实例；开关在 service 层（repo），这里只管可用集合。
#[derive(Default)]
pub struct ToolRegistry {
    tools: BTreeMap<String, Arc<dyn Tool>>,
}

impl ToolRegistry {
    /// 空表。
    pub fn new() -> Self {
        Self::default()
    }

    /// 注册（同名覆盖，后到者胜——功能/MCP 工具可替换内置）。
    pub fn register(&mut self, tool: Arc<dyn Tool>) {
        self.tools.insert(tool.name().to_string(), tool);
    }

    /// 按名取工具。
    pub fn get(&self, name: &str) -> Option<Arc<dyn Tool>> {
        self.tools.get(name).cloned()
    }

    /// 全部工具（名字序）。
    pub fn all(&self) -> Vec<Arc<dyn Tool>> {
        self.tools.values().cloned().collect()
    }

    /// 是否包含。
    pub fn contains(&self, name: &str) -> bool {
        self.tools.contains_key(name)
    }
}

impl ToolRegistry {
    /// 深快照：把当前表复制一份给内核线程独占使用。
    pub fn snapshot(src: &ToolRegistry) -> ToolRegistry {
        let mut reg = ToolRegistry::new();
        for (k, v) in src.tools.iter() {
            reg.tools.insert(k.clone(), v.clone());
        }
        reg
    }
}
