//! 用量统计：历史会话里已记账的 usage 一次性回填进日表。

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use wb_kernel::{Entry, Msg};

use crate::Services;

/// 回填进度标记（settings KV；存在即已回填过）。
const BACKFILL_KEY: &str = "stats_backfill_done";

/// (provider, model, day) → (input, output, cache_read, cache_write)
type UsageAgg = BTreeMap<(String, String, String), (i64, i64, i64, i64)>;

/// 启动时回填全部历史会话用量；重复调用幂等。
pub fn backfill(sv: &Arc<Services>) {
    if sv.repo.settings_get(BACKFILL_KEY).ok().flatten().is_some() {
        return;
    }
    let mut files = Vec::new();
    collect_jsonl(&sv.data_dir.join("sessions"), 0, &mut files);
    for path in files {
        backfill_one(sv, &path);
    }
    let _ = sv.repo.settings_set(BACKFILL_KEY, "1");
}

/// 回填单个会话文件：assistant 消息上的 usage 按 (provider, model, day) 聚合。
/// 走原始条目而非回放——压缩前的消息也已被计费，同样要入账。
fn backfill_one(sv: &Arc<Services>, path: &Path) {
    let Ok(sf) = wb_kernel::SessionFile::load(path) else {
        return;
    };
    let mut agg: UsageAgg = BTreeMap::new();
    for w in &sf.entries {
        let Entry::Message {
            message:
                Msg::Assistant {
                    usage: Some(u),
                    provider_id,
                    model_id,
                    timestamp,
                    ..
                },
        } = &w.payload
        else {
            continue;
        };
        let key = (
            provider_id.clone(),
            model_id.clone(),
            wb_repo::usage::day_key_of_ms(*timestamp),
        );
        let slot = agg.entry(key).or_default();
        slot.0 += u.input;
        slot.1 += u.output;
        slot.2 += u.cache_read;
        slot.3 += u.cache_write;
    }
    for ((provider_id, model_id, day), (i, o, cr, cw)) in agg {
        let _ = sv.repo.usage_add(
            &day,
            &provider_id,
            &model_id,
            &sf.header.session_id,
            i,
            o,
            cr,
            cw,
            0.0,
        );
    }
}

/// 递归收集目录下全部 .jsonl（深度限 4，覆盖各工作区子目录与子代理会话）。
fn collect_jsonl(dir: &Path, depth: usize, out: &mut Vec<PathBuf>) {
    if depth > 4 {
        return;
    }
    let Ok(entries) = std::fs::read_dir(dir) else {
        return;
    };
    for e in entries.flatten() {
        let p = e.path();
        if p.is_dir() {
            collect_jsonl(&p, depth + 1, out);
        } else if p.extension().is_some_and(|x| x == "jsonl") {
            out.push(p);
        }
    }
}
