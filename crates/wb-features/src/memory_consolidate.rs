//! 知识整合：把散落的记忆去重、合并、提炼成更少的条目。
//! 记忆库只增不减就会堆满互相重复的碎片——检索召回一堆近似内容，模型也分不清哪条更新。
//! 整合按「同一类别 → 逐对判重 → 合并」跑，合并后条目变少、内容更全。

use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use crate::memory_store::MemoryStore;

/// 当前毫秒时间戳。
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// 一轮整合的结果。
pub struct ConsolidateReport {
    /// 扫描的条目数
    pub scanned: usize,
    /// 合并掉的条目数
    pub merged: usize,
    /// 保留的条目数
    pub remaining: usize,
    /// 整合后的类别条数（类别 → 条数）
    pub by_kind: Vec<(String, i64)>,
}

/// 相似度：两段文本的字符级词元集合交并比。
/// 中文按字切、西文按词切。字级切分对中文短句更稳——「项目用 pnpm 管理依赖」
/// 与「这个项目依赖由 pnpm 管」这类改写能对上，分词切错就完全对不上。
fn similarity(a: &str, b: &str) -> f64 {
    let ta = tokens(a);
    let tb = tokens(b);
    if ta.is_empty() || tb.is_empty() {
        return 0.0;
    }
    let inter = ta.intersection(&tb).count() as f64;
    let union = ta.union(&tb).count() as f64;
    inter / union
}

/// 分词：西文按词，中文按字；过滤停用词。
fn tokens(text: &str) -> std::collections::BTreeSet<String> {
    text.to_lowercase()
        .split(|c: char| !c.is_alphanumeric())
        .filter(|w| !w.is_empty())
        .filter(|w| !is_stop_word(w))
        .flat_map(|w| {
            // 连续汉字拆成单字；西文词原样保留。
            if w.chars()
                .all(|c| c.is_ascii_alphabetic() || c.is_ascii_digit())
            {
                vec![w.to_string()]
            } else {
                w.chars().map(|c| c.to_string()).collect()
            }
        })
        .filter(|w| !w.is_empty())
        .collect()
}

/// 特征词：西文词与数字（包名、命令、路径）。
/// 纯中文句子没有特征词，所以两段纯中文若没有共同特征词就不合并——
/// 「我喜欢猫」与「我不喜欢狗」字面重合很高，但说的不是一回事。
fn distinctive_tokens(text: &str) -> std::collections::BTreeSet<String> {
    text.to_lowercase()
        .split(|c: char| !c.is_alphanumeric())
        .filter(|w| w.len() >= 2 && !is_stop_word(w))
        .filter(|w| {
            w.chars()
                .any(|c| c.is_ascii_alphabetic() || c.is_ascii_digit())
        })
        .map(|w| w.to_string())
        .collect()
}

/// 是否停用词。
fn is_stop_word(w: &str) -> bool {
    STOP_WORDS.contains(&w)
}

/// 停用词（高頻虚词，不参与判重）。
const STOP_WORDS: &[&str] = &[
    "the", "and", "for", "with", "that", "this", "from", "are", "was", "you", "your", "not", "的",
    "了", "是", "在", "和", "与", "或", "也", "就", "都", "而", "及", "着", "过",
];

/// 相似度阈值：高于它就判为重复并合并。
const MERGE_THRESHOLD: f64 = 0.5;

/// 单次整合最多处理的条目数（控制扫描成本）。
const MAX_SCAN: usize = 300;

/// 整合参数：来自 memory 功能配置，缺省值与常量一致。
#[derive(Debug, Clone, Copy)]
pub struct ConsolidateOpts {
    /// 相似度达到多少才合并（0.1..=1.0）
    pub merge_threshold: f64,
    /// 记忆条数超过它才自动整合（0 = 关闭自动整合）
    pub auto_threshold: usize,
}

impl Default for ConsolidateOpts {
    /// 与历史常量一致：0.5 阈值、40 条触发。
    fn default() -> Self {
        Self {
            merge_threshold: MERGE_THRESHOLD,
            auto_threshold: 40,
        }
    }
}

impl ConsolidateOpts {
    /// 从功能配置取值。
    pub fn from_config(cfg: &crate::config::MemoryConfig) -> Self {
        Self {
            merge_threshold: cfg.memory_merge_threshold,
            auto_threshold: cfg.memory_auto_threshold,
        }
    }
}

/// 整合记忆库：同类别内逐对判重，重复的合并成一条。
/// 合并方向：保留最近更新的那条（较早的内容并进去），删掉较早的那条——
/// 最近更新的代表当前认知，历史陈述并进去不丢信息。
pub fn consolidate(store: &Arc<MemoryStore>, opts: ConsolidateOpts) -> ConsolidateReport {
    let mut scanned = 0usize;
    let mut merged = 0usize;
    let kinds: Vec<String> = store
        .count_by_kind()
        .unwrap_or_default()
        .into_iter()
        .map(|(k, _)| k)
        .collect();
    for kind in kinds {
        let items = store.list_by_kind(&kind, MAX_SCAN).unwrap_or_default();
        scanned += items.len();
        // 已合并掉的条目会从库里消失，先记下存活集合避免重复处理。
        let mut alive: Vec<String> = items.iter().map(|(id, _)| id.clone()).collect();
        for i in 0..items.len() {
            for j in (i + 1)..items.len() {
                let (id_a, text_a) = &items[i];
                let (id_b, text_b) = &items[j];
                if !alive.contains(id_b) {
                    continue;
                }
                if similarity(text_a, text_b) < opts.merge_threshold {
                    continue;
                }
                // 特征词门槛：没有共同特征词就不合并。
                // 纯中文句子（「我喜欢猫」与「我不喜欢狗」）字面重合很高，
                // 但没有共同特征词说明它们说的不是一件事。
                let da = distinctive_tokens(text_a);
                let db = distinctive_tokens(text_b);
                if !da.is_empty() && !db.is_empty() && da.intersection(&db).next().is_none() {
                    continue;
                }
                if store.merge(id_a, id_b, now_ms()).is_ok() {
                    alive.retain(|id| id != id_b);
                    merged += 1;
                }
            }
        }
    }
    let by_kind = store.count_by_kind().unwrap_or_default();
    let remaining = by_kind.iter().map(|(_, n)| *n as usize).sum();
    ConsolidateReport {
        scanned,
        merged,
        remaining,
        by_kind,
    }
}

/// 自动整合：条数超过阈值时跑一轮，返回是否真的整合了。
pub fn auto_consolidate(
    store: &Arc<MemoryStore>,
    opts: ConsolidateOpts,
) -> Option<ConsolidateReport> {
    if opts.auto_threshold == 0 || store.total().ok()? < opts.auto_threshold as i64 {
        return None;
    }
    let report = consolidate(store, opts);
    if report.merged == 0 {
        None
    } else {
        Some(report)
    }
}
