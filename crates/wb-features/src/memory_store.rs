//! 记忆存储：SQLite FTS5 持久库 + 历史会话 JSONL 的内存 FTS 检索。

use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::UNIX_EPOCH;

use rusqlite::{params, Connection};
use serde_json::Value;

use wb_pkg::{AppError, Result};

use crate::FeatureCtx;

/// 建表语句：正文表 + FTS5 外部内容虚表 + 触发器同步。
const SCHEMA_SQL: &str = "
CREATE TABLE IF NOT EXISTS memory(
    memory_id TEXT PRIMARY KEY,
    memory_kind TEXT,
    memory_text TEXT,
    memory_created_at INTEGER,
    memory_updated_at INTEGER
);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
    memory_text, content='memory', content_rowid='rowid', tokenize='unicode61'
);
CREATE TRIGGER IF NOT EXISTS memory_ai AFTER INSERT ON memory BEGIN
    INSERT INTO memory_fts(rowid, memory_text) VALUES (new.rowid, new.memory_text);
END;
CREATE TRIGGER IF NOT EXISTS memory_ad AFTER DELETE ON memory BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, memory_text)
    VALUES ('delete', old.rowid, old.memory_text);
END;
CREATE TRIGGER IF NOT EXISTS memory_au AFTER UPDATE OF memory_text ON memory BEGIN
    INSERT INTO memory_fts(memory_fts, rowid, memory_text)
    VALUES ('delete', old.rowid, old.memory_text);
    INSERT INTO memory_fts(rowid, memory_text) VALUES (new.rowid, new.memory_text);
END;
";

/// 单次会话检索最多扫的文件数（按 mtime 取最近）。
const MAX_SESSION_FILES: usize = 50;
/// 判 JSONL 形态读取的文件头字节数。
const HEAD_BYTES: usize = 2048;

/// 持久记忆库：单连接串行访问；所有调用一律走 spawn_blocking。
pub struct MemoryStore {
    /// SQLite 连接（Mutex 保证串行）
    conn: Mutex<Connection>,
}

impl MemoryStore {
    /// 打开（不存在则建库建表），库文件在功能目录 memory.db。
    pub fn open(ctx: &FeatureCtx) -> Result<Self> {
        let dir = ctx.feature_dir();
        std::fs::create_dir_all(&dir).map_err(|e| AppError::wrap("1002", "建功能目录失败", e))?;
        let conn = Connection::open(dir.join("memory.db"))
            .map_err(|e| AppError::wrap("2002", "记忆库打开失败", e))?;
        conn.execute_batch(SCHEMA_SQL).map_err(db_err)?;
        Ok(Self {
            conn: Mutex::new(conn),
        })
    }

    /// 保存一条记忆：同 kind 同文本只刷新时间戳，否则新增；返回 (id, 是否新增)。
    pub fn save(&self, kind: &str, text: &str, now: i64) -> Result<(String, bool)> {
        let conn = self.conn.lock().unwrap();
        let existing: Option<String> = conn
            .query_row(
                "SELECT memory_id FROM memory WHERE memory_kind = ?1 AND memory_text = ?2",
                params![kind, text],
                |r| r.get(0),
            )
            .ok();
        if let Some(id) = existing {
            conn.execute(
                "UPDATE memory SET memory_updated_at = ?1 WHERE memory_id = ?2",
                params![now, id],
            )
            .map_err(db_err)?;
            return Ok((id, false));
        }
        let id = new_memory_id();
        conn.execute(
            "INSERT INTO memory(memory_id, memory_kind, memory_text, memory_created_at, memory_updated_at)
             VALUES (?1, ?2, ?3, ?4, ?4)",
            params![id, kind, text, now],
        )
        .map_err(db_err)?;
        Ok((id, true))
    }

    /// 删除一条记忆；返回是否真的删了。
    pub fn remove(&self, memory_id: &str) -> Result<bool> {
        let conn = self.conn.lock().unwrap();
        let n = conn
            .execute(
                "DELETE FROM memory WHERE memory_id = ?1",
                params![memory_id],
            )
            .map_err(db_err)?;
        Ok(n > 0)
    }

    /// 双路检索：FTS5 词面 + LIKE 逐字，按 RRF 融合后取前 limit 条。
    /// 中文查询基本走不到 FTS（unicode61 把整句当一个 token），LIKE 路是命中保底；
    /// 逐词各跑一条 LIKE，命中多个词的条目自然累加得分，排得更准。
    pub fn search(&self, query: &str, limit: usize) -> Result<Vec<MemoryHit>> {
        if query.trim().is_empty() {
            return Ok(Vec::new());
        }
        let conn = self.conn.lock().unwrap();
        let cand = (limit as i64) * 4;
        // 第一路：FTS5 词面命中，rank 升序
        let fts: Vec<i64> = {
            let mut stmt = conn
                .prepare(
                    "SELECT memory_fts.rowid FROM memory_fts
                     WHERE memory_fts MATCH ?1 ORDER BY rank LIMIT ?2",
                )
                .map_err(db_err)?;
            let rows = stmt
                .query_map(params![crate::fts::match_query(query), cand], |r| {
                    r.get::<_, i64>(0)
                })
                .map_err(db_err)?;
            rows.flatten().collect()
        };
        // 第二路：逐词 LIKE 子串命中
        let mut routes: Vec<Vec<i64>> = vec![fts];
        for term in query.split_whitespace().filter(|t| !t.trim().is_empty()) {
            let mut stmt = conn
                .prepare(
                    "SELECT rowid FROM memory WHERE memory_text LIKE ?1 ESCAPE '\\'
                     ORDER BY memory_updated_at DESC LIMIT ?2",
                )
                .map_err(db_err)?;
            let hits = stmt
                .query_map(
                    params![format!("%{}%", crate::fts::escape_like(term)), cand],
                    |r| r.get::<_, i64>(0),
                )
                .map_err(db_err)?
                .flatten()
                .collect::<Vec<i64>>();
            routes.push(hits);
        }
        let scores = crate::fts::rrf_scores(&routes);
        let mut out = Vec::new();
        for rowid in crate::fts::rank_desc(&scores).into_iter().take(limit) {
            let mut stmt = conn
                .prepare(
                    "SELECT memory_id, memory_kind, memory_text, memory_updated_at
                     FROM memory WHERE rowid = ?1",
                )
                .map_err(db_err)?;
            out.push(
                stmt.query_row(params![rowid], |r| {
                    Ok(MemoryHit {
                        memory_id: r.get(0)?,
                        memory_kind: r.get(1)?,
                        memory_summary: summarize(&r.get::<_, String>(2)?),
                        memory_updated_at: r.get(3)?,
                    })
                })
                .map_err(db_err)?,
            );
        }
        Ok(out)
    }

    /// 列出某类别的全部记忆（按更新时间倒序）。
    pub fn list_by_kind(&self, kind: &str, limit: usize) -> Result<Vec<(String, String)>> {
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn
            .prepare(
                "SELECT memory_id, memory_text FROM memory WHERE memory_kind = ?1
                 ORDER BY memory_updated_at DESC LIMIT ?2",
            )
            .map_err(db_err)?;
        let rows = stmt
            .query_map(params![kind, limit as i64], |r| Ok((r.get(0)?, r.get(1)?)))
            .map_err(db_err)?;
        rows.collect::<std::result::Result<Vec<_>, _>>()
            .map_err(db_err)
    }

    /// 合并两条记忆：把 from 的正文并入 into，并删掉 from。
    ///
    /// 用于知识整合：两条说的是同一件事时保留一条，把另一条的内容并进去。
    pub fn merge(&self, into_id: &str, from_id: &str, now: i64) -> Result<()> {
        let conn = self.conn.lock().unwrap();
        let into_text: Option<String> = conn
            .query_row(
                "SELECT memory_text FROM memory WHERE memory_id = ?1",
                params![into_id],
                |r| r.get(0),
            )
            .ok();
        let Some(into_text) = into_text else {
            return Err(AppError::new(
                wb_domain::error::tool::MEMORY_NOT_FOUND,
                "待合并的记忆不存在",
            ));
        };
        let from_text: Option<String> = conn
            .query_row(
                "SELECT memory_text FROM memory WHERE memory_id = ?1",
                params![from_id],
                |r| r.get(0),
            )
            .ok();
        let Some(from_text) = from_text else {
            return Err(AppError::new(
                wb_domain::error::tool::MEMORY_NOT_FOUND,
                "待合并的记忆不存在",
            ));
        };
        let merged = if into_text.contains(&from_text) {
            into_text
        } else if from_text.contains(&into_text) {
            from_text
        } else {
            format!("{into_text}\n{from_text}")
        };
        conn.execute(
            "UPDATE memory SET memory_text = ?1, memory_updated_at = ?2 WHERE memory_id = ?3",
            params![merged, now, into_id],
        )
        .map_err(db_err)?;
        conn.execute("DELETE FROM memory WHERE memory_id = ?1", params![from_id])
            .map_err(db_err)?;
        Ok(())
    }

    /// 统计各类别的条数。
    pub fn count_by_kind(&self) -> Result<Vec<(String, i64)>> {
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn
            .prepare("SELECT memory_kind, COUNT(*) FROM memory GROUP BY memory_kind")
            .map_err(db_err)?;
        let rows = stmt
            .query_map([], |r| Ok((r.get(0)?, r.get(1)?)))
            .map_err(db_err)?;
        rows.collect::<std::result::Result<Vec<_>, _>>()
            .map_err(db_err)
    }

    /// 记忆总条数。
    pub fn total(&self) -> Result<i64> {
        let conn = self.conn.lock().unwrap();
        conn.query_row("SELECT COUNT(*) FROM memory", [], |r| r.get(0))
            .map_err(db_err)
    }
}

/// 记忆命中。
pub struct MemoryHit {
    /// 记忆 id（MEM_ 前缀）
    pub memory_id: String,
    /// 类别：memory / user / project / failure
    pub memory_kind: String,
    /// 正文摘要（前 120 字符）
    pub memory_summary: String,
    /// 更新时间（毫秒）
    pub memory_updated_at: i64,
}

/// 历史会话命中。
pub struct SessionHit {
    /// 会话文件名（不含目录）
    pub session_file: String,
    /// 命中行号（1 起）
    pub line_no: usize,
    /// 消息正文摘要（前 120 字符）
    pub line_summary: String,
}

/// 在历史会话 JSONL 里全文检索；会话目录不存在返回空。
/// 与记忆库同一套双路纪律：FTS 词面 + 逐词子串召回再融合，否则中文查不到。
pub fn search_sessions(sessions_dir: &Path, query: &str, limit: usize) -> Result<Vec<SessionHit>> {
    if query.trim().is_empty() || !sessions_dir.is_dir() {
        return Ok(Vec::new());
    }
    let mut files: Vec<(i64, PathBuf)> = Vec::new();
    collect_jsonl(sessions_dir, 0, &mut files);
    files.sort_by_key(|(mtime, _)| std::cmp::Reverse(*mtime));
    files.truncate(MAX_SESSION_FILES);

    let conn = Connection::open_in_memory().map_err(db_err)?;
    conn.execute_batch("CREATE VIRTUAL TABLE line_fts USING fts5(text, tokenize='unicode61');")
        .map_err(db_err)?;
    // 行元数据与正文按下标同 rowid 对齐，命中后直接回查，免二次 SQL
    let mut meta: Vec<(String, usize)> = Vec::new();
    let mut texts: Vec<String> = Vec::new();
    for (_, path) in &files {
        let Some((prefer_payload, lines)) = read_session_lines(path) else {
            continue;
        };
        let name = path
            .file_name()
            .map(|n| n.to_string_lossy().into_owned())
            .unwrap_or_default();
        for (line_no, line) in lines {
            let Some(text) = message_text(&line, prefer_payload) else {
                continue;
            };
            conn.execute(
                "INSERT INTO line_fts(rowid, text) VALUES (?1, ?2)",
                params![meta.len() as i64, text],
            )
            .map_err(db_err)?;
            meta.push((name.clone(), line_no));
            texts.push(text);
        }
    }
    let cand = limit * 4;
    let fts: Vec<i64> = {
        let mut stmt = conn
            .prepare("SELECT rowid FROM line_fts WHERE line_fts MATCH ?1 ORDER BY rank LIMIT ?2")
            .map_err(db_err)?;
        let rows = stmt
            .query_map(params![crate::fts::match_query(query), cand as i64], |r| {
                r.get::<_, i64>(0)
            })
            .map_err(db_err)?;
        rows.flatten().collect()
    };
    // 逐词子串召回：行数量级小，直接在内存里比，不另建一张 LIKE 表
    let mut routes: Vec<Vec<i64>> = vec![fts];
    for term in query.split_whitespace() {
        let needle = term.to_lowercase();
        let hits: Vec<i64> = texts
            .iter()
            .enumerate()
            .filter(|(_, t)| t.to_lowercase().contains(&needle))
            .map(|(idx, _)| idx as i64)
            .take(cand)
            .collect();
        routes.push(hits);
    }
    let scores = crate::fts::rrf_scores(&routes);
    let out = crate::fts::rank_desc(&scores)
        .into_iter()
        .take(limit)
        .filter_map(|rowid| {
            let idx = rowid.max(0) as usize;
            let (session_file, line_no) = meta.get(idx)?.clone();
            Some(SessionHit {
                session_file,
                line_no,
                line_summary: summarize(texts.get(idx).map(|s| s.as_str()).unwrap_or_default()),
            })
        })
        .collect();
    Ok(out)
}

/// 递归收集 .jsonl 会话文件（按工作区分层）；深度限 4。
fn collect_jsonl(dir: &Path, depth: u8, out: &mut Vec<(i64, PathBuf)>) {
    if depth > 4 {
        return;
    }
    let Ok(entries) = std::fs::read_dir(dir) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if path.is_dir() {
            collect_jsonl(&path, depth + 1, out);
        } else if path.extension().map(|e| e == "jsonl").unwrap_or(false) {
            let mtime = entry
                .metadata()
                .and_then(|m| m.modified())
                .ok()
                .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
                .map(|d| d.as_millis() as i64)
                .unwrap_or(0);
            out.push((mtime, path));
        }
    }
}

/// 打开会话文件：先读头部判形态，再回卷逐行读全；打不开返回 None。
fn read_session_lines(path: &Path) -> Option<(bool, Vec<(usize, String)>)> {
    use std::io::{BufRead, BufReader, Read, Seek, SeekFrom};
    let mut file = std::fs::File::open(path).ok()?;
    let mut head = [0u8; HEAD_BYTES];
    let n = file.read(&mut head).unwrap_or(0);
    let prefer_payload = String::from_utf8_lossy(&head[..n]).contains("\"payload\"");
    file.seek(SeekFrom::Start(0)).ok()?;
    let lines: Vec<(usize, String)> = BufReader::new(file)
        .lines()
        .enumerate()
        .filter_map(|(i, l)| l.ok().map(|line| (i + 1, line)))
        .collect();
    Some((prefer_payload, lines))
}

/// 提取一条 JSONL 行的消息正文；嵌套包装与内核扁平两种载体都试，读不出返回 None。
fn message_text(line: &str, prefer_payload: bool) -> Option<String> {
    let v: Value = serde_json::from_str(line).ok()?;
    let nested_msg = extract_text(at_path(&v, "message.message.content"));
    let nested_pay = extract_text(at_path(&v, "payload.message.content"));
    let flat_msg = extract_text(at_path(&v, "message.content"));
    let flat_pay = extract_text(at_path(&v, "payload.content"));
    if prefer_payload {
        nested_pay.or(flat_pay).or(nested_msg).or(flat_msg)
    } else {
        nested_msg.or(flat_msg).or(nested_pay).or(flat_pay)
    }
}

/// 按点分路径逐层取值。
fn at_path<'a>(v: &'a Value, path: &str) -> Option<&'a Value> {
    let mut cur = v;
    for key in path.split('.') {
        cur = cur.get(key)?;
    }
    Some(cur)
}

/// content 形态：字符串直取；数组拼接 text 块；其余不算正文。
fn extract_text(content: Option<&Value>) -> Option<String> {
    match content? {
        Value::String(s) => Some(s.clone()),
        Value::Array(blocks) => {
            let parts: Vec<String> = blocks
                .iter()
                .filter_map(|b| {
                    let text = b.get("text").and_then(|v| v.as_str())?;
                    let is_text_block = b
                        .get("type")
                        .and_then(|v| v.as_str())
                        .map(|t| t == "text")
                        .unwrap_or(true);
                    is_text_block.then(|| text.to_string())
                })
                .collect();
            (!parts.is_empty()).then(|| parts.join("\n"))
        }
        _ => None,
    }
}

/// 摘要：压平空白取前 120 字符，超长加省略号。
pub(crate) fn summarize(text: &str) -> String {
    let flat = text.split_whitespace().collect::<Vec<_>>().join(" ");
    let chars: Vec<char> = flat.chars().collect();
    if chars.len() <= 120 {
        flat
    } else {
        format!("{}…", chars[..120].iter().collect::<String>())
    }
}

/// 生成 MEM_ 前缀的记忆 id。
fn new_memory_id() -> String {
    wb_pkg::ulid::new_id(wb_pkg::Prefix::Mem)
}

/// rusqlite 错误统一转 AppError。
fn db_err(e: rusqlite::Error) -> AppError {
    AppError::wrap("2002", "记忆库操作失败", e)
}
