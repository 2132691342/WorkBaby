//! 工具体系：Tool 契约 + 注册表 + 内置 7 工具。扩展工具（功能 / MCP）经同一契约并入。

pub mod builtin;
pub mod exec;
pub mod registry;
pub mod truncate;

pub use registry::{Tool, ToolAnnotations, ToolCall, ToolOutput, ToolRegistry};
