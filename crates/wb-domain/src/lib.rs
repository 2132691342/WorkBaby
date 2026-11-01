//! 纯类型层：DO/DTO/REQ/RESP/VO 与错误码常量。字段一律 `xxx_` 业务前缀，serde snake_case。

pub mod chat;
pub mod error;
pub mod events;
pub mod feature;
pub mod git;
pub mod mcp;
pub mod messages;
pub mod provider;
pub mod resource;
pub mod runtime;
pub mod session;
pub mod settings;
pub mod skill;
pub mod stats;
pub mod subagent;
pub mod tool;
