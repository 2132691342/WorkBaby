//! SQLite 持久层：连接与手写迁移、设置 KV、三类开关、会话索引、用量统计。

pub mod db;
pub mod session_index;
pub mod settings;
pub mod toggle;
pub mod usage;

pub use db::Repo;
