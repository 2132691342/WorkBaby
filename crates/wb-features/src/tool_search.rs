//! tool_search：让模型按需找工具，而不是一开始就把几十个工具定义摊给它。
//!
//! 全部工具定义常驻上下文要几千 token。与其省这笔钱，不如给模型一个
//! 「找工具」的手段：它先搜，搜到的工具在下一轮真的出现。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use std::collections::HashMap;

use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

/// 一次返回多少个工具。
const DEFAULT_LIMIT: usize = 8;
/// 上限：再多就淹没上下文了。
const MAX_LIMIT: usize = 20;

// ---- 检索 ----

/// 一个待检索的工具文档：工具名 + 检索用的文本。
pub struct ToolDocument {
    /// 工具名
    pub name: String,
    /// 检索文本（描述 + 参数说明拼成的可读文本）
    pub text: String,
}

/// 一条命中：工具名 + 分数。
#[derive(Debug, Clone)]
pub struct ToolMatch {
    /// 工具名
    pub name: String,
    /// 相关性分数（越大越相关）
    pub score: f64,
}

/// 英文停用词。
const STOP_WORDS: [&str; 18] = [
    "a", "an", "and", "are", "as", "at", "be", "by", "for", "from", "in", "is", "it", "of", "on",
    "or", "the", "to",
];

/// 停用词：出现太普遍的分词对排序没有区分度。
fn is_stop_word(term: &str) -> bool {
    STOP_WORDS.contains(&term)
        || matches!(term, "的" | "了" | "和" | "是" | "在" | "我" | "你" | "他")
}

/// 极简词干化：只做够用的几处变形，让 `searches` 能命中 `search`。
fn stem(term: &str) -> String {
    if term.len() > 4 && term.ends_with("ies") {
        return format!("{}y", &term[..term.len() - 3]);
    }
    if term.len() > 4
        && (term.ends_with("ches") || term.ends_with("shes") || term.ends_with("sses"))
    {
        return term[..term.len() - 2].to_string();
    }
    if term.len() > 3 && term.ends_with('s') && !term.ends_with("ss") {
        return term[..term.len() - 1].to_string();
    }
    term.to_string()
}

/// 分词：英文按空白与标点切，中文按字切。
/// 中文不分词时整句会成为一个 token，改写句就完全对不上，所以按字切。
fn tokenize(text: &str) -> Vec<String> {
    let mut out = Vec::new();
    let mut ascii = String::new();
    for ch in text.chars() {
        if ch.is_ascii_alphanumeric() {
            ascii.push(ch.to_ascii_lowercase());
            continue;
        }
        if !ascii.is_empty() {
            out.push(std::mem::take(&mut ascii));
        }
        if is_cjk(ch) {
            out.push(ch.to_string());
        }
    }
    if !ascii.is_empty() {
        out.push(ascii);
    }
    out.into_iter()
        .map(|t| stem(&t))
        .filter(|t| !t.is_empty() && !is_stop_word(t))
        .collect()
}

/// 是否是中日韩字符（含全角标点）。
fn is_cjk(ch: char) -> bool {
    matches!(ch as u32,
        0x3000..=0x30FF | 0x3400..=0x4DBF | 0x4E00..=0x9FFF | 0xF900..=0xFAFF | 0xAC00..=0xD7AF
            | 0xFF00..=0xFFEF)
}

/// BM25 打分排序：查询词在工具名与描述里出现得越多、越靠前，分数越高。
/// 参数 k1 控制词频饱和，b 控制长文档惩罚。
pub fn rank(query: &str, documents: &[ToolDocument], limit: usize) -> Vec<ToolMatch> {
    let query_terms = tokenize(query);
    if query_terms.is_empty() || documents.is_empty() {
        return Vec::new();
    }
    // 文档词频表：工具名命中比描述命中更值钱，权重给三倍
    let mut doc_terms: Vec<HashMap<String, usize>> = Vec::with_capacity(documents.len());
    let mut doc_len: Vec<usize> = Vec::with_capacity(documents.len());
    for d in documents {
        let mut terms: HashMap<String, usize> = HashMap::new();
        for t in tokenize(&d.name) {
            *terms.entry(t).or_default() += 3;
        }
        for t in tokenize(&d.text) {
            *terms.entry(t).or_default() += 1;
        }
        doc_len.push(terms.values().sum());
        doc_terms.push(terms);
    }
    let avg_len = (doc_len.iter().sum::<usize>() as f64 / documents.len() as f64).max(1.0);
    // 文档频率：某个查询词出现在几份文档里（决定该词的区分度）
    let mut df: HashMap<&str, usize> = HashMap::new();
    for terms in &doc_terms {
        for t in terms.keys() {
            if query_terms.iter().any(|q| q == t) {
                *df.entry(t.as_str()).or_default() += 1;
            }
        }
    }
    const K1: f64 = 1.2;
    const B: f64 = 0.75;
    let n = documents.len() as f64;
    let mut out: Vec<ToolMatch> = Vec::new();
    for (i, d) in documents.iter().enumerate() {
        let mut score = 0.0;
        for q in &query_terms {
            let tf = doc_terms[i].get(q).copied().unwrap_or(0) as f64;
            if tf == 0.0 {
                continue;
            }
            let n_q = df.get(q.as_str()).copied().unwrap_or(0) as f64;
            let idf = ((n - n_q + 0.5) / (n_q + 0.5) + 1.0).ln();
            score +=
                idf * (tf * (K1 + 1.0)) / (tf + K1 * (1.0 - B + B * doc_len[i] as f64 / avg_len));
        }
        if score > 0.0 {
            out.push(ToolMatch {
                name: d.name.clone(),
                score,
            });
        }
    }
    out.sort_by(|a, b| {
        b.score
            .partial_cmp(&a.score)
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    out.truncate(limit);
    out
}

// ---- 工具 ----

/// 一个待检索的工具条目。
pub struct CatalogEntry {
    /// 工具名
    pub name: String,
    /// 描述
    pub description: String,
    /// 参数 schema 的可读文本
    pub parameters_text: String,
}

/// tool_search 工具。
pub struct ToolSearchTool {
    /// 全部可加载的工具：名字 + 描述 + 参数说明
    catalog: Vec<CatalogEntry>,
}

impl ToolSearchTool {
    /// 构造：给一份可加载工具的清单。
    pub fn new(catalog: Vec<CatalogEntry>) -> Self {
        Self { catalog }
    }
}

#[async_trait]
impl Tool for ToolSearchTool {
    fn name(&self) -> &str {
        "tool_search"
    }

    fn label(&self) -> &str {
        "搜索工具"
    }

    fn description(&self) -> &str {
        "按用途搜索可用工具并加载它们。参数：query(必填，想做什么的自然语言)、\
         limit(可选，默认 8，上限 20)。\
         搜到的工具会在下一轮请求里出现，之后就能直接调用。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "想做什么（如「查网页」「改文件」「跑测试」）" },
                "limit": { "type": "integer", "description": "返回几个工具（默认 8，上限 20）", "minimum": 1, "maximum": 20 }
            },
            "required": ["query"]
        })
    }

    fn annotations(&self) -> ToolAnnotations {
        ToolAnnotations {
            read_only: true,
            destructive: false,
        }
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &std::path::Path,
        ctx: &wb_tools::ToolCtx,
    ) -> Result<ToolOutput> {
        let query = args["query"]
            .as_str()
            .map(|s| s.trim())
            .filter(|s| !s.is_empty())
            .ok_or_else(|| AppError::new("1001", "query 不能为空"))?;
        let limit = args["limit"].as_u64().unwrap_or(DEFAULT_LIMIT as u64) as usize;
        let limit = limit.clamp(1, MAX_LIMIT);

        let documents: Vec<ToolDocument> = self
            .catalog
            .iter()
            .map(|e| ToolDocument {
                name: e.name.clone(),
                text: format!("{} {}", e.description, e.parameters_text),
            })
            .collect();
        let hits = rank(query, &documents, limit);
        if hits.is_empty() {
            let names: Vec<&str> = self.catalog.iter().map(|e| e.name.as_str()).collect();
            return Ok(ToolOutput::plain(
                format!(
                    "没找到匹配「{query}」的工具。当前可用：{}",
                    names.join(" / ")
                ),
                false,
                0,
            ));
        }

        // 加载命中的工具：加载后下一轮请求里才有它们的定义
        let loaded: Vec<String> = hits.iter().map(|h| h.name.clone()).collect();
        ctx.load_tools(&loaded);

        let mut text = String::new();
        for h in &hits {
            let Some(e) = self.catalog.iter().find(|e| e.name == h.name) else {
                continue;
            };
            text.push_str(&format!(
                "- {}：{}\n",
                e.name,
                e.description.lines().next().unwrap_or("").trim()
            ));
        }
        text.push_str(&format!(
            "\n已加载 {} 个工具，下一轮即可直接调用。",
            loaded.len()
        ));
        Ok(ToolOutput::plain(text, false, 0))
    }
}
