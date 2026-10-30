//! 模型接入层：ModelClient 契约 + wire API 实现 + 配置/凭据存储。

pub mod client;
pub mod config;
pub mod openai;
pub mod types;

pub use client::create_client;
pub use types::*;
