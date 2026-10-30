//! 知识库：FTS5 存正文 + 分段索引，RRF 融合多路召回（词面 + 逐字）。

use std::path::Path;
use std::sync::Mutex;

use rusqlite::{params, Connection};
use serde_json::Value;

use wb_pkg::{AppError, Result};

/// 建表语句：正文表 + chunk 表 + chunk 上的 FTS5 外部内容虚表 + 触发器同步。
const SCHEMA_SQL: &str = "
CREATE TABLE IF NOT EXISTS doc(
    doc_id TEXT PRIMARY KEY,
    doc_source TEXT,
    doc_title TEXT,
    doc_text TEXT,
    doc_created_at INTEGER
);
CREATE TABLE IF NOT EXISTS chunk(
    chunk_id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_doc_id TEXT,
    chunk_ordinal INTEGER,
    chunk_text TEXT
);
CREATE INDEX IF NOT EXISTS chunk_doc_idx ON chunk(chunk_doc_id);
CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts USING fts5(
    chunk_text, content='chunk', content_rowid='rowid', tokenize='unicode61'
);
CREATE TRIGGER IF NOT EXISTS chunk_ai AFTER INSERT ON chunk BEGIN
    INSERT INTO chunk_fts(rowid, chunk_text) VALUES (new.rowid, new.chunk_text);
END;
CREATE TRIGGER IF NOT EXISTS chunk_ad AFTER DELETE ON chunk BEGIN
    INSERT INTO chunk_fts(chunk_fts, rowid, chunk_text) VALUES ('delete', old.rowid, old.chunk_text);
END;
CREATE TRIGGER IF NOT EXISTS chunk_au AFTER UPDATE OF chunk_text ON chunk BEGIN
    INSERT INTO chunk_fts(chunk_fts, rowid, chunk_text) VALUES ('delete', old.rowid, old.chunk_text);
    INSERT INTO chunk_fts(rowid, chunk_text) VALUES (new.rowid, new.chunk_text);
END;
";

/// 知识库命中。
pub struct ChunkHit {
    /// 文档 id（DOC_ 前缀）
    pub chunk_doc_id: String,
    /// 文档来源（路径或 URL）
    pub chunk_source: String,
    /// 文档标题
    pub chunk_title: String,
    /// 命中段序号
    pub chunk_ordinal: i64,
    /// 命中段正文摘要
    pub chunk_excerpt: String,
    /// 融合得分（RRF）
    pub chunk_score: f64,
}

/// 知识库：单连接串行访问；所有调用一律走 spawn_blocking。
pub struct KnowledgeStore {
    /// SQLite 连接（Mutex 保证串行）
    conn: Mutex<Connection>,
}

impl KnowledgeStore {
    /// 打开（不存在则建库建表），库文件在功能目录 knowledge.db。
    pub fn open(dir: &Path) -> Result<Self> {
        std::fs::create_dir_all(dir).map_err(|e| AppError::wrap("1002", "建知识库目录失败", e))?;
        let conn = Connection::open(dir.join("knowledge.db"))
            .map_err(|e| AppError::wrap("2002", "知识库打开失败", e))?;
        conn.execute_batch(SCHEMA_SQL).map_err(db_err)?;
        Ok(Self {
            conn: Mutex::new(conn),
        })
    }

    /// 收录一份文档：正文入 FTS5，按段落切块入 chunk 表。
    pub fn index(&self, source: &str, title: &str, text: &str, now: i64) -> Result<String> {
        let doc_id = format!("DOC_{}", wb_pkg::ulid::raw_ulid());
        let conn = self.conn.lock().unwrap();
        conn.execute(
            "INSERT INTO doc(doc_id, doc_source, doc_title, doc_text, doc_created_at)
             VALUES (?1, ?2, ?3, ?4, ?5)",
            params![doc_id, source, title, text, now],
        )
        .map_err(db_err)?;
        for (i, chunk) in split_chunks(text).into_iter().enumerate() {
            conn.execute(
                "INSERT INTO chunk(chunk_doc_id, chunk_ordinal, chunk_text) VALUES (?1, ?2, ?3)",
                params![doc_id, i as i64, chunk],
            )
            .map_err(db_err)?;
        }
        Ok(doc_id)
    }

    /// 检索：词面 FTS5 与逐字 LIKE 两路召回，按 RRF 融合后取前 limit 条。
    pub fn search(&self, query: &str, limit: usize) -> Result<Vec<ChunkHit>> {
        if query.trim().is_empty() {
            return Ok(Vec::new());
        }
        let conn = self.conn.lock().unwrap();
        // 第一路：FTS5 词面命中，按 rank 升序取候选
        let mut fts: Vec<(i64, i64)> = Vec::new();
        {
            let mut stmt = conn
                .prepare(
                    "SELECT chunk.rowid, chunk.chunk_ordinal
                     FROM chunk JOIN chunk_fts ON chunk_fts.rowid = chunk.rowid
                     WHERE chunk_fts MATCH ?1 ORDER BY rank LIMIT ?2",
                )
                .map_err(db_err)?;
            let rows = stmt
                .query_map(params![fts_match_query(query), (limit * 4) as i64], |r| {
                    Ok((r.get::<_, i64>(0)?, r.get::<_, i64>(1)?))
                })
                .map_err(db_err)?;
            for r in rows.flatten() {
                fts.push(r);
            }
        }
        // 第二路：逐字 LIKE 命中（覆盖缩写与专有名词）
        let mut like: Vec<(i64, i64)> = Vec::new();
        {
            let mut stmt = conn
                .prepare(
                    "SELECT chunk.rowid, chunk.chunk_ordinal FROM chunk
                     WHERE chunk.chunk_text LIKE ?1 ESCAPE '\\' ORDER BY chunk.rowid LIMIT ?2",
                )
                .map_err(db_err)?;
            let rows = stmt
                .query_map(
                    params![format!("%{}%", escape_like(query)), (limit * 4) as i64],
                    |r| Ok((r.get::<_, i64>(0)?, r.get::<_, i64>(1)?)),
                )
                .map_err(db_err)?;
            for r in rows.flatten() {
                like.push(r);
            }
        }
        // RRF 融合：每路第 k 位的贡献为 1/(k+60)
        let mut scores: std::collections::HashMap<i64, (f64, i64, i64)> =
            std::collections::HashMap::new();
        for (rank, (rowid, ordinal)) in fts.iter().enumerate() {
            let e = scores.entry(*rowid).or_insert((0.0, *ordinal, 0));
            e.0 += 1.0 / (60.0 + rank as f64 + 1.0);
            e.2 = 1;
        }
        for (rank, (rowid, ordinal)) in like.iter().enumerate() {
            let e = scores.entry(*rowid).or_insert((0.0, *ordinal, 0));
            e.0 += 1.0 / (60.0 + rank as f64 + 1.0);
        }
        if scores.is_empty() {
            return Ok(Vec::new());
        }
        // 取得分最高的若干 chunk，回查文档元信息
        let mut ranked: Vec<(i64, f64, i64, i64)> = scores
            .into_iter()
            .map(|(rowid, (score, ordinal, _))| (rowid, score, ordinal, 0))
            .collect();
        ranked.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        ranked.truncate(limit);
        let mut out = Vec::with_capacity(ranked.len());
        for (rowid, score, ordinal, _) in ranked {
            let mut stmt = conn
                .prepare(
                    "SELECT chunk.chunk_doc_id, chunk.chunk_text, doc.doc_source, doc.doc_title
                     FROM chunk JOIN doc ON doc.doc_id = chunk.chunk_doc_id
                     WHERE chunk.rowid = ?1",
                )
                .map_err(db_err)?;
            let row = stmt
                .query_row(params![rowid], |r| {
                    Ok(ChunkHit {
                        chunk_doc_id: r.get(0)?,
                        chunk_source: r.get(2)?,
                        chunk_title: r.get(3)?,
                        chunk_ordinal: ordinal,
                        chunk_excerpt: excerpt(&r.get::<_, String>(1)?),
                        chunk_score: score,
                    })
                })
                .ok();
            if let Some(hit) = row {
                out.push(hit);
            }
        }
        Ok(out)
    }

    /// 文档总数（界面统计用）。
    pub fn doc_count(&self) -> Result<i64> {
        let conn = self.conn.lock().unwrap();
        conn.query_row("SELECT COUNT(*) FROM doc", [], |r| r.get(0))
            .map_err(db_err)
    }
}

/// 按空行切块：每段一块，超长段再按句号切。
fn split_chunks(text: &str) -> Vec<String> {
    let mut out: Vec<String> = Vec::new();
    for para in text.split("\n\n") {
        let para = para.trim();
        if para.is_empty() {
            continue;
        }
        if para.chars().count() <= 1200 {
            out.push(para.to_string());
            continue;
        }
        for part in para.split(". ").filter(|s| !s.trim().is_empty()) {
            out.push(format!("{}.", part.trim_end_matches('.')));
        }
    }
    out
}

/// 摘要：压平空白取前 240 字符。
fn excerpt(text: &str) -> String {
    let flat = text.split_whitespace().collect::<Vec<_>>().join(" ");
    let chars: Vec<char> = flat.chars().collect();
    if chars.len() <= 240 {
        flat
    } else {
        format!("{}…", chars[..240].iter().collect::<String>())
    }
}

/// 用户查询转 FTS5 MATCH 串：逐词加引号，避免撞上 FTS5 语法字符。
fn fts_match_query(query: &str) -> String {
    query
        .split_whitespace()
        .map(|term| format!("\"{}\"", term.replace('"', "\"\"")))
        .collect::<Vec<_>>()
        .join(" ")
}

/// LIKE 模式里的特殊字符转义。
fn escape_like(text: &str) -> String {
    text.replace('\\', "\\\\")
        .replace('%', "\\%")
        .replace('_', "\\_")
}

/// rusqlite 错误统一转 AppError。
fn db_err(e: rusqlite::Error) -> AppError {
    AppError::wrap("2002", "知识库操作失败", e)
}

/// 供工具层解析 JSON 入参的辅助（避免每个工具重复写 Value 取值）。
pub fn text_arg(args: &Value, key: &str) -> String {
    args.get(key)
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .trim()
        .to_string()
}
