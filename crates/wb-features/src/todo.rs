//! 待办清单：模型增删改查与依赖标记，状态存功能文件（跨压缩与重启存活）。
//! 存储取舍：Tool 契约拿不到会话句柄，所以用工作区级文件而不是会话 custom 条目。

use std::path::Path;
use std::sync::Arc;
use std::time::{Instant, SystemTime, UNIX_EPOCH};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use crate::FeatureCtx;

/// 单条待办。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TodoItem {
    /// 条目 id（TODO_{ULID}，唯一标识，供 blocked_by 引用）
    pub todo_id: String,
    /// 展示序号（清单内 1 起稠密编号，删除后重排）
    #[serde(default)]
    pub todo_seq: u32,
    /// 内容
    pub todo_text: String,
    /// pending / in_progress / completed
    pub todo_status: String,
    /// 依赖的条目 id
    pub todo_blocked_by: Vec<String>,
    /// 创建时间（毫秒）
    pub todo_created_at: i64,
    /// 更新时间（毫秒）
    pub todo_updated_at: i64,
}

/// 当前毫秒时间戳。
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// 重排展示序号：按现有顺序给 1 起稠密编号，删除后列表不跳号。
fn renumber(items: &mut [TodoItem]) {
    for (i, t) in items.iter_mut().enumerate() {
        t.todo_seq = i as u32 + 1;
    }
}

/// 按 id 或展示序号找条目；不存在报 7004。
fn find<'a>(items: &'a mut [TodoItem], id: &str) -> Result<&'a mut TodoItem> {
    let by_seq = id.parse::<u32>().ok();
    items
        .iter_mut()
        .find(|t| t.todo_id == id || Some(t.todo_seq) == by_seq)
        .ok_or_else(|| AppError::new(toolcode::ARG_INVALID, format!("{id} 不存在")))
}

/// 把 id 或展示序号归一成条目真 id。
/// 两种写法混着用会让依赖图断链：成环检测与删除清理都只按 id 比，
/// 存成序号的那条引用就静默地查不到。
fn resolve_id(items: &[TodoItem], id: &str) -> Option<String> {
    let by_seq = id.parse::<u32>().ok();
    items
        .iter()
        .find(|t| t.todo_id == id || Some(t.todo_seq) == by_seq)
        .map(|t| t.todo_id.clone())
}

/// 依赖清单归一：逐条解成真 id 并去重；解不出即报悬空。
fn normalize_deps(items: &[TodoItem], deps: &[String]) -> Result<Vec<String>> {
    let mut out: Vec<String> = Vec::with_capacity(deps.len());
    for d in deps {
        let real = resolve_id(items, d)
            .ok_or_else(|| AppError::new(toolcode::ARG_INVALID, format!("依赖的 {d} 不存在")))?;
        if !out.contains(&real) {
            out.push(real);
        }
    }
    Ok(out)
}

/// from 加一条到 target 的依赖会不会成环（沿依赖图向上走）。
fn would_cycle(items: &[TodoItem], from: &str, target: &str) -> bool {
    let mut stack = vec![target.to_string()];
    let mut seen = std::collections::BTreeSet::new();
    while let Some(id) = stack.pop() {
        if id == from {
            return true;
        }
        if !seen.insert(id.clone()) {
            continue;
        }
        if let Some(t) = items.iter().find(|t| t.todo_id == id) {
            stack.extend(t.todo_blocked_by.iter().cloned());
        }
    }
    false
}

/// 校验已归一的依赖：自环 / 成环都拒（悬空在归一那步就报过了）。
fn ensure_deps(items: &[TodoItem], id: &str, deps: &[String]) -> Result<()> {
    for d in deps {
        if d == id {
            return Err(AppError::new(toolcode::ARG_INVALID, "不能依赖自己"));
        }
        if would_cycle(items, id, d) {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("依赖 {d} 会成环"),
            ));
        }
    }
    Ok(())
}

/// 待办存储：一份 JSON 文件，读改写串行。
pub struct TodoStore {
    path: std::path::PathBuf,
}

impl TodoStore {
    /// 按 FeatureCtx 定位存储文件。
    pub fn new(ctx: &FeatureCtx) -> Self {
        Self {
            path: ctx.feature_dir().join("todo.json"),
        }
    }

    /// 读全量（文件缺失 = 空）；载入即重排展示序号。
    pub fn load(&self) -> Vec<TodoItem> {
        let mut items: Vec<TodoItem> = std::fs::read_to_string(&self.path)
            .ok()
            .and_then(|t| serde_json::from_str(&t).ok())
            .unwrap_or_default();
        renumber(&mut items);
        items
    }

    /// 读改写一次；闭包报错则不落盘（写临时文件再重命名，断电不留半截）。
    pub fn mutate(
        &self,
        f: impl FnOnce(&mut Vec<TodoItem>) -> Result<()>,
    ) -> Result<Vec<TodoItem>> {
        let mut items = self.load();
        f(&mut items)?;
        wb_pkg::fsutil::ensure_dir(self.path.parent().unwrap_or(Path::new(".")))
            .map_err(|e| AppError::wrap("1002", "建待办目录失败", e))?;
        let text = serde_json::to_string_pretty(&items)
            .map_err(|e| AppError::new("1002", format!("待办序列化失败：{e}")))?;
        wb_pkg::fsutil::write_atomic(&self.path, text.as_bytes())
            .map_err(|e| AppError::wrap("1002", "写待办文件失败", e))?;
        renumber(&mut items);
        Ok(items)
    }
}

/// todo 工具。
pub struct TodoTool {
    store: Arc<TodoStore>,
    ctx: FeatureCtx,
}

impl TodoTool {
    /// 装配。
    pub fn new(store: Arc<TodoStore>, ctx: FeatureCtx) -> Self {
        Self { store, ctx }
    }

    /// 模型可读的清单渲染。
    fn render(items: &[TodoItem]) -> String {
        if items.is_empty() {
            return "（清单为空）".into();
        }
        let mark = |s: &str| match s {
            "completed" => "[x]",
            "in_progress" => "[~]",
            _ => "[ ]",
        };
        items
            .iter()
            .map(|t| {
                let blocked = if t.todo_blocked_by.is_empty() {
                    String::new()
                } else {
                    format!("（blocked by {}）", t.todo_blocked_by.join(", "))
                };
                format!(
                    "{}. {} {}{}",
                    t.todo_seq,
                    mark(&t.todo_status),
                    t.todo_text,
                    blocked
                )
            })
            .collect::<Vec<_>>()
            .join("\n")
    }

    /// 状态面板行（chat:widget）。
    fn widget_lines(items: &[TodoItem]) -> Vec<String> {
        items
            .iter()
            .map(|t| {
                let mark = match t.todo_status.as_str() {
                    "completed" => "[x]",
                    "in_progress" => "[~]",
                    _ => "[ ]",
                };
                format!("{mark} {} {}", t.todo_seq, t.todo_text)
            })
            .collect()
    }

    /// 应用一次变更；返回新清单。
    fn apply(
        store: &TodoStore,
        op: &str,
        id: &str,
        text: &str,
        status: &str,
        deps: Vec<String>,
        now: i64,
    ) -> Result<Vec<TodoItem>> {
        match op {
            "list" => Ok(store.load()),
            "add" => {
                if text.is_empty() {
                    return Err(AppError::new(toolcode::ARG_INVALID, "add 需要 text"));
                }
                store.mutate(|items| {
                    // 先归一再入库：存下去的依赖一律是真 id，后面的图运算才靠得住
                    let deps = normalize_deps(items, &deps)?;
                    let item = TodoItem {
                        todo_id: wb_pkg::ulid::new_id(wb_pkg::ulid::Prefix::Todo),
                        todo_seq: 0,
                        todo_text: text.to_string(),
                        todo_status: "pending".into(),
                        todo_blocked_by: deps.clone(),
                        todo_created_at: now,
                        todo_updated_at: now,
                    };
                    ensure_deps(items, &item.todo_id, &deps)?;
                    items.push(item);
                    Ok(())
                })
            }
            "update" => store.mutate(|items| {
                // 目标与依赖都先归一成 id，否则序号写法会绕开自环/成环判断
                let target = resolve_id(items, id)
                    .ok_or_else(|| AppError::new(toolcode::ARG_INVALID, format!("{id} 不存在")))?;
                let deps = if deps.is_empty() {
                    Vec::new()
                } else {
                    let normalized = normalize_deps(items, &deps)?;
                    ensure_deps(items, &target, &normalized)?;
                    normalized
                };
                let t = find(items, &target)?;
                if !text.is_empty() {
                    t.todo_text = text.to_string();
                }
                if !deps.is_empty() {
                    t.todo_blocked_by = deps;
                }
                t.todo_updated_at = now;
                Ok(())
            }),
            "status" => {
                if !matches!(status, "pending" | "in_progress" | "completed") {
                    return Err(AppError::new(toolcode::ARG_INVALID, "status 取值非法"));
                }
                store.mutate(|items| {
                    let t = find(items, id)?;
                    t.todo_status = status.to_string();
                    t.todo_updated_at = now;
                    Ok(())
                })
            }
            "remove" => store.mutate(|items| {
                let by_seq = id.parse::<u32>().ok();
                let doomed: Vec<String> = items
                    .iter()
                    .filter(|t| t.todo_id == id || Some(t.todo_seq) == by_seq)
                    .map(|t| t.todo_id.clone())
                    .collect();
                if doomed.is_empty() {
                    return Err(AppError::new(toolcode::ARG_INVALID, format!("{id} 不存在")));
                }
                items.retain(|t| !doomed.contains(&t.todo_id));
                // 依赖它的条目清掉悬空引用
                for t in items.iter_mut() {
                    t.todo_blocked_by.retain(|d| !doomed.contains(d));
                }
                Ok(())
            }),
            other => Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("不认识的动作 {other}"),
            )),
        }
    }
}

#[async_trait]
impl Tool for TodoTool {
    fn name(&self) -> &str {
        "todo"
    }

    fn label(&self) -> &str {
        "待办清单"
    }

    fn description(&self) -> &str {
        "维护一份跨上下文压缩存活的待办清单。多步任务开始时先 add 分解步骤；进行中及时 status 更新；步骤间有先后依赖时用 blocked_by 标记，被依赖的没完成前不要做下一步。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "op": { "type": "string", "enum": ["add", "update", "remove", "status", "list"], "description": "动作" },
                "todo_id": { "type": "string", "description": "条目 id 或序号（add 时不需要）" },
                "text": { "type": "string", "description": "add/update 的内容" },
                "status": { "type": "string", "enum": ["pending", "in_progress", "completed"], "description": "status 动作的目标状态" },
                "blocked_by": { "type": "array", "items": { "type": "string" }, "description": "add/update 时设置的依赖 id" }
            },
            "required": ["op"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let op = args
            .get("op")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let id = args
            .get("todo_id")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let text = args
            .get("text")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let status = args
            .get("status")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let deps: Vec<String> = args
            .get("blocked_by")
            .and_then(|v| v.as_array())
            .map(|a| {
                a.iter()
                    .filter_map(|x| x.as_str().map(|s| s.to_string()))
                    .collect()
            })
            .unwrap_or_default();
        let store = self.store.clone();
        // 文件读写小而快，丢阻塞线程池避免占 runtime
        let items = tokio::task::spawn_blocking(move || {
            TodoTool::apply(&store, &op, &id, &text, &status, deps, now_ms())
        })
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))??;
        (self.ctx.emit)(
            "",
            "chat:widget",
            json!({ "key": "todo", "lines": Self::widget_lines(&items) }),
        );
        Ok(ToolOutput {
            text: Self::render(&items),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
