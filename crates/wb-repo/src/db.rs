//! 连接打开 + user_version 步进迁移。表结构只增不删（AGENTS.md §2.11）。

use std::path::Path;
use std::sync::Mutex;

use rusqlite::Connection;

use wb_pkg::{AppError, Result};

/// 仓库句柄：进程内共享的单连接。
pub struct Repo {
    /// SQLite 连接（WAL 模式；所有写经这里串行）
    pub conn: Mutex<Connection>,
}

impl Repo {
    /// 打开（不存在则创建）并执行迁移。
    pub fn open(path: &Path) -> Result<Self> {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| AppError::wrap("2004", "数据目录不可写", e))?;
        }
        let conn =
            Connection::open(path).map_err(|e| AppError::wrap("2002", "数据库打开失败", e))?;
        conn.pragma_update(None, "journal_mode", "WAL")
            .map_err(|e| AppError::wrap("2002", "设置 WAL 失败", e))?;
        conn.pragma_update(None, "busy_timeout", 5000)
            .map_err(|e| AppError::wrap("2002", "设置 busy_timeout 失败", e))?;
        migrate(&conn)?;
        Ok(Self {
            conn: Mutex::new(conn),
        })
    }
}

/// 按版本步进执行迁移；每个版本一段 SQL。
fn migrate(conn: &Connection) -> Result<()> {
    let v: i64 = conn
        .query_row("PRAGMA user_version", [], |r| r.get(0))
        .map_err(|e| AppError::wrap("2002", "读取 user_version 失败", e))?;
    const LATEST: i64 = 1;
    if v >= LATEST {
        return Ok(());
    }
    conn.execute_batch(
        "BEGIN;
        CREATE TABLE IF NOT EXISTS settings_kv (
            key   TEXT PRIMARY KEY,
            value TEXT NOT NULL,
            updated_at INTEGER NOT NULL DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS session_index (
            session_id      TEXT PRIMARY KEY,
            session_name    TEXT NOT NULL DEFAULT '',
            session_workspace TEXT NOT NULL DEFAULT '',
            session_provider TEXT NOT NULL DEFAULT '',
            session_model   TEXT NOT NULL DEFAULT '',
            session_message_count INTEGER NOT NULL DEFAULT 0,
            session_last_active_at INTEGER NOT NULL DEFAULT 0,
            session_summary TEXT NOT NULL DEFAULT ''
        );
        CREATE TABLE IF NOT EXISTS usage_daily (
            day TEXT NOT NULL,
            provider_id TEXT NOT NULL,
            model_id TEXT NOT NULL,
            session_id TEXT NOT NULL,
            input_tokens INTEGER NOT NULL DEFAULT 0,
            output_tokens INTEGER NOT NULL DEFAULT 0,
            cache_read INTEGER NOT NULL DEFAULT 0,
            cache_write INTEGER NOT NULL DEFAULT 0,
            cost_usd REAL NOT NULL DEFAULT 0,
            PRIMARY KEY (day, provider_id, model_id, session_id)
        );
        CREATE TABLE IF NOT EXISTS feature_toggle (
            feature_code TEXT PRIMARY KEY,
            enabled INTEGER NOT NULL DEFAULT 0,
            config TEXT NOT NULL DEFAULT '{}'
        );
        CREATE TABLE IF NOT EXISTS skill_toggle (
            skill_name TEXT PRIMARY KEY,
            enabled INTEGER NOT NULL DEFAULT 1
        );
        CREATE TABLE IF NOT EXISTS tool_toggle (
            tool_name TEXT PRIMARY KEY,
            enabled INTEGER NOT NULL DEFAULT 1
        );
        COMMIT;",
    )
    .map_err(|e| AppError::wrap("2002", "迁移执行失败", e))?;
    conn.pragma_update(None, "user_version", LATEST)
        .map_err(|e| AppError::wrap("2002", "写入版本失败", e))?;
    Ok(())
}

/// rusqlite 错误 → AppError 的统一转换。
pub fn db_err(e: rusqlite::Error) -> AppError {
    AppError::wrap("2002", "数据库操作失败", e)
}
