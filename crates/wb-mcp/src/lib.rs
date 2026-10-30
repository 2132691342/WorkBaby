//! MCP 客户端编排：配置（config.rs）+ 连接池（pool.rs）。工具并入注册表由 service 完成。

pub mod config;
pub mod pool;

pub use config::{McpConfig, Scope};
pub use pool::Pool;
