//! fff 内部件：frecency SQLite 存储、工作区遍历与 nucleo 打分原语。

use std::collections::HashMap;
use std::path::Path;
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

use nucleo::pattern::Pattern;
use nucleo::{Matcher, Utf32Str};
use rusqlite::Connection;
use tokio_util::sync::CancellationToken;
use wb_domain::error::{config as cfgcode, tool as toolcode};
use wb_pkg::{AppError, Result};

use crate::FeatureCtx;

/// 单文件参与内容搜索的体积上限（字节），超过直接跳过。
pub(super) const MAX_FILE_BYTES: usize = 1024 * 1024;
/// 单文件最多返回的命中行数。
pub(super) const LINES_PER_FILE: usize = 3;
/// 输出行内容的最大字符数。
pub(super) const LINE_CLIP_CHARS: usize = 300;
/// frecency bonus 的上限（排序权重最多放大到 1.5 倍）。
pub(super) const BONUS_CAP: f64 = 0.5;
/// 遍历每多少个条目检查一次取消。
const CANCEL_CHECK_EVERY: usize = 64;
/// 遍历时整目录跳过的黑名单（.git 是版本库，其余是构建产物）。
const SKIP_DIRS: [&str; 4] = [".git", "node_modules", "target", "dist"];

/// 当前毫秒时间戳。
pub(super) fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// frecency 得分 = count / (age_hours + 2)^0.6：次数线性增益，时间按 0.6 次幂衰减，+2 压低新条目并避免除零。
pub fn frecency_score(count: i64, last_at_ms: i64, now: i64) -> f64 {
    let age_hours = (now - last_at_ms).max(0) as f64 / 3_600_000.0;
    count as f64 / (age_hours + 2.0).powf(0.6)
}

/// 取路径的 frecency bonus（[0, 0.5)）：0.5 * f / (f + 1) 单调收敛，永不越过上限；未命中为 0。
pub fn bonus_from(frec: &HashMap<String, (i64, i64)>, path: &str, now: i64) -> f64 {
    let Some((count, last_at)) = frec.get(path) else {
        return 0.0;
    };
    let f = frecency_score(*count, *last_at, now);
    BONUS_CAP * f / (f + 1.0)
}

/// 极简 glob（* 与 ?）转大小写不敏感正则；文件名过滤不需要完整 glob 语义。
pub(super) fn glob_regex(glob: &str) -> Result<regex::Regex> {
    let mut pat = String::from("(?i)^");
    for c in glob.chars() {
        match c {
            '*' => pat.push_str(".*"),
            '?' => pat.push('.'),
            c => pat.push_str(&regex::escape(&c.to_string())),
        }
    }
    pat.push('$');
    regex::Regex::new(&pat).map_err(|e| AppError::wrap(toolcode::ARG_INVALID, "glob 非法", e))
}

/// 行内容按字符截断。
pub(super) fn clip(line: &str, max_chars: usize) -> String {
    line.chars().take(max_chars).collect()
}

/// 遍历工作区文件（尊重 .gitignore），回调收 (绝对路径, 正斜杠相对路径)；返回是否因取消中止。
pub fn walk_files(root: &Path, cancel: &CancellationToken, mut f: impl FnMut(&Path, &str)) -> bool {
    let mut visited = 0usize;
    let mut walker = ignore::WalkBuilder::new(root);
    walker.filter_entry(|e| {
        // 只剪目录，避免误杀恰好同名的顶层文件
        !(e.file_type().is_some_and(|t| t.is_dir())
            && SKIP_DIRS.contains(&e.file_name().to_string_lossy().as_ref()))
    });
    for entry in walker.build().flatten() {
        visited += 1;
        if visited.is_multiple_of(CANCEL_CHECK_EVERY) && cancel.is_cancelled() {
            return true;
        }
        let p = entry.path();
        if !p.is_file() {
            continue;
        }
        // 统一正斜杠：Windows 反斜杠拿不到 nucleo 的路径分隔奖励，模型拼接也更容易
        let rel = p
            .strip_prefix(root)
            .unwrap_or(p)
            .display()
            .to_string()
            .replace('\\', "/");
        f(p, &rel);
    }
    cancel.is_cancelled()
}

/// 逐行模糊打分取分最高的前 LINES_PER_FILE 行；返回 (文件最高分, [(行号, 行文本)])，无命中为 None。
pub(super) fn best_lines(
    text: &str,
    pattern: &Pattern,
    matcher: &mut Matcher,
) -> Option<(u32, Vec<(usize, String)>)> {
    let mut buf: Vec<char> = Vec::new();
    let mut hits: Vec<(usize, u32, &str)> = text
        .lines()
        .enumerate()
        .filter_map(|(i, line)| {
            let s = pattern.score(Utf32Str::new(line, &mut buf), matcher)?;
            (s > 0).then_some((i, s, line))
        })
        .collect();
    if hits.is_empty() {
        return None;
    }
    // 先按分取 top，再按行号恢复输出顺序
    hits.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
    hits.truncate(LINES_PER_FILE);
    let best = hits[0].1;
    hits.sort_by_key(|(i, _, _)| *i);
    Some((
        best,
        hits.into_iter()
            .map(|(i, _, l)| (i + 1, l.to_string()))
            .collect(),
    ))
}

/// frecency 存储：工作区级 SQLite（feature_dir\fff.db），记录文件命中计数与查询历史。
pub struct FrecencyStore {
    /// SQLite 连接（互斥串行；工具在阻塞线程池里读写）
    conn: Mutex<Connection>,
}

impl FrecencyStore {
    /// 打开（不存在则建库建表）。
    pub fn new(ctx: &FeatureCtx) -> Result<Self> {
        let path = ctx.feature_dir().join("fff.db");
        std::fs::create_dir_all(path.parent().unwrap_or(Path::new(".")))
            .map_err(|e| AppError::wrap(cfgcode::DATA_DIR_READONLY, "建 fff 数据目录失败", e))?;
        let conn = Connection::open(&path)
            .map_err(|e| AppError::wrap(cfgcode::DB_FAILED, "打开 fff.db 失败", e))?;
        Self::migrate(&conn)?;
        Ok(Self {
            conn: Mutex::new(conn),
        })
    }

    /// user_version 步进迁移：v1 建 frecency 与 query_history 两张表。
    fn migrate(conn: &Connection) -> Result<()> {
        let v: i64 = conn
            .query_row("PRAGMA user_version", [], |r| r.get(0))
            .map_err(|e| AppError::wrap(cfgcode::DB_FAILED, "读 user_version 失败", e))?;
        if v >= 1 {
            return Ok(());
        }
        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS frecency (
                path    TEXT PRIMARY KEY,
                count   INTEGER NOT NULL DEFAULT 0,
                last_at INTEGER NOT NULL DEFAULT 0
            );
            CREATE TABLE IF NOT EXISTS query_history (
                query TEXT NOT NULL,
                at    INTEGER NOT NULL DEFAULT 0
            );
            PRAGMA user_version = 1;",
        )
        .map_err(|e| AppError::wrap(cfgcode::DB_FAILED, "fff.db 建表失败", e))
    }

    /// 全量读出 frecency 表（path → (count, last_at)）；表小，一次读入做内存加权。
    pub fn snapshot(&self) -> HashMap<String, (i64, i64)> {
        let conn = self.conn.lock().unwrap_or_else(|e| e.into_inner());
        let Ok(mut stmt) = conn.prepare("SELECT path, count, last_at FROM frecency") else {
            return HashMap::new();
        };
        stmt.query_map([], |r| Ok((r.get(0)?, r.get(1)?, r.get(2)?)))
            .map(|rows| {
                rows.filter_map(|r| r.ok())
                    .map(|(p, c, t)| (p, (c, t)))
                    .collect()
            })
            // 读失败当空表，搜索不因统计库故障中断
            .unwrap_or_default()
    }

    /// 命中 +1 计数并刷新时间戳（新路径按 count=1 插入）。
    pub fn record_hit(&self, path: &str, now: i64) -> Result<()> {
        let conn = self.conn.lock().unwrap_or_else(|e| e.into_inner());
        conn.execute(
            "INSERT INTO frecency(path, count, last_at) VALUES(?1, 1, ?2)
             ON CONFLICT(path) DO UPDATE SET count = count + 1, last_at = ?2",
            rusqlite::params![path, now],
        )
        .map_err(|e| AppError::wrap(cfgcode::DB_FAILED, "写 frecency 失败", e))?;
        Ok(())
    }

    /// 记一条查询历史。
    pub fn record_query(&self, query: &str, now: i64) -> Result<()> {
        let conn = self.conn.lock().unwrap_or_else(|e| e.into_inner());
        conn.execute(
            "INSERT INTO query_history(query, at) VALUES(?1, ?2)",
            rusqlite::params![query, now],
        )
        .map_err(|e| AppError::wrap(cfgcode::DB_FAILED, "写查询历史失败", e))?;
        Ok(())
    }
}
