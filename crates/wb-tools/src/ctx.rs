//! 工具运行时上下文：谁发起调用、往哪个会话回话、执行过程往哪里吐。

use std::sync::Arc;

/// 增量输出回调：`(输出片段)`，内核把它转成 `ToolProgress` 事件。
pub type ProgressFn = Arc<dyn Fn(&str) + Send + Sync>;

/// 工具加载回调：`(工具名列表)`，请求把工具的定义加进下一轮请求。
/// 工具表是「按需加载」时，模型搜到工具后要真的让它出现在下一次请求里。
pub type LoadToolsFn = Arc<dyn Fn(&[String]) + Send + Sync>;

/// 一次工具调用所属的运行时环境。
/// 功能工具发事件必须写进**发起调用的那个会话**（全局帧会串台），`run_id` 区分同会话的不同轮；
/// `progress` 是执行过程的增量出口（内核已注入并节流，工具侧直接用）。
#[derive(Clone, Default)]
pub struct ToolCtx {
    /// 发起调用的会话 id；空 = 没有会话归属（非对话上下文）
    pub session_id: String,
    /// 发起调用的那一轮运行 id
    pub run_id: String,
    /// 增量输出回调；None = 调用方不要过程回显
    pub progress: Option<ProgressFn>,
    /// 工具加载回调；None = 工具表固定，不支持按需加载
    pub load_tools: Option<LoadToolsFn>,
}

impl std::fmt::Debug for ToolCtx {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ToolCtx")
            .field("session_id", &self.session_id)
            .field("run_id", &self.run_id)
            .field("progress", &self.progress.is_some())
            .field("load_tools", &self.load_tools.is_some())
            .finish()
    }
}

impl ToolCtx {
    /// 造一个带归属的上下文（无过程回显）。
    pub fn new(session_id: impl Into<String>, run_id: impl Into<String>) -> Self {
        Self {
            session_id: session_id.into(),
            run_id: run_id.into(),
            progress: None,
            load_tools: None,
        }
    }

    /// 挂上过程回显回调。
    pub fn with_progress(mut self, progress: ProgressFn) -> Self {
        self.progress = Some(progress);
        self
    }

    /// 挂上工具加载回调：工具在执行中请求把别的工具加进下一轮。
    pub fn with_loader(mut self, loaded: std::sync::Arc<std::sync::Mutex<Vec<String>>>) -> Self {
        self.load_tools = Some(std::sync::Arc::new(move |names: &[String]| {
            let mut guard = loaded.lock().unwrap();
            for n in names {
                if !guard.contains(n) {
                    guard.push(n.clone());
                }
            }
        }));
        self
    }

    /// 吐一段执行过程；没挂回调时什么也不做。
    pub fn emit_progress(&self, chunk: &str) {
        if let Some(cb) = &self.progress {
            cb(chunk);
        }
    }

    /// 请求把指定工具加进下一轮请求的工具表。
    /// 没挂回调时什么也不做——固定工具表的场合不支持按需加载。
    pub fn load_tools(&self, names: &[String]) {
        if let Some(cb) = &self.load_tools {
            cb(names);
        }
    }
}
