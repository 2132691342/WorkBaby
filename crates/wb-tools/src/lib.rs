//! 工具层：契约、注册表、内置工具与共享执行器。内核只认 `Arc<dyn Tool>`。

pub mod archive;
pub mod ctx;
pub mod difftext;
pub mod exec;
pub mod job;
pub mod registry;
pub mod truncate;

pub mod builtin;

pub use ctx::{ProgressFn, ToolCtx};
pub use registry::{
    DiffLine, Tool, ToolAnnotations, ToolCall, ToolImage, ToolOutput, ToolRegistry,
};
