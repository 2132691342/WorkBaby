//! FTS5 检索公共件：MATCH 串构造、LIKE 转义、双路 RRF 融合。
//! 中文词间没有空格，unicode61 会把整句切成一个 token，
//! 因此凡走 FTS 的库都必须再配一条 LIKE 逐字召回，两路融合才有命中。

use std::collections::HashMap;

/// RRF 平滑常数：名次靠后的贡献衰减到该基准附近。
const RRF_K: f64 = 60.0;

/// 用户查询转 FTS5 MATCH 串：逐词加引号，避免撞上 FTS5 语法字符。
pub fn match_query(query: &str) -> String {
    query
        .split_whitespace()
        .map(|term| format!("\"{}\"", term.replace('"', "\"\"")))
        .collect::<Vec<_>>()
        .join(" ")
}

/// LIKE 模式里的特殊字符转义（需配合 SQL 的 ESCAPE '\'）。
pub fn escape_like(text: &str) -> String {
    text.replace('\\', "\\\\")
        .replace('%', "\\%")
        .replace('_', "\\_")
}

/// 多路召回融合成一张分数表：每路内的顺序即名次，同键跨路累加。
pub fn rrf_scores<K>(routes: &[Vec<K>]) -> HashMap<K, f64>
where
    K: Eq + std::hash::Hash + Copy,
{
    let mut out: HashMap<K, f64> = HashMap::new();
    for route in routes {
        for (rank, key) in route.iter().enumerate() {
            *out.entry(*key).or_insert(0.0) += 1.0 / (RRF_K + rank as f64 + 1.0);
        }
    }
    out
}

/// 分数表降序排名：同分按主键升序，保证结果稳定可复现。
pub fn rank_desc<K>(scores: &HashMap<K, f64>) -> Vec<K>
where
    K: Eq + std::hash::Hash + Copy + Ord,
{
    let mut keys: Vec<K> = scores.keys().copied().collect();
    keys.sort_by(|a, b| {
        scores[b]
            .partial_cmp(&scores[a])
            .unwrap_or(std::cmp::Ordering::Equal)
            .then(a.cmp(b))
    });
    keys
}
