//! 业务编排层：对话 run 管理、KernelEvent→SSE 翻译、系统提示词组装、目录与装配。
//! 依赖全部能力域，但不引 axum / tauri。

pub mod agents;
pub mod approval;
pub mod chat;
pub mod chat_events;
pub mod chat_model;
pub mod context;
pub mod export;
pub mod feature_config;
pub mod feature_state;
pub mod feature_write;
pub mod features;
pub mod files;
pub mod git;
pub mod loadout;
pub mod mcp;
pub mod misc;
pub mod prompt;
pub mod providers;
pub mod resources;
pub mod review;
pub mod runtime;
pub mod skills;
pub mod store;
pub mod task;
pub mod usage;
pub mod workspace;

use std::path::PathBuf;
use std::sync::Arc;

use tokio::sync::mpsc;

/// 事件出口：service 唯一的 SSE 注入点（wb-api 的 Hub 实现它）。
pub trait Emitter: Send + Sync {
    /// 发一条 SSE 事件（event 名 + JSON 载荷）；session_id 为空 = 全局通道。
    fn emit(&self, session_id: &str, event: &str, data: serde_json::Value);
}

/// 一轮运行的上下文快照：task 工具据此知道「当前父会话是谁、用哪个模型」。
#[derive(Clone)]
pub struct RunScope {
    /// 父会话 id
    pub session_id: String,
    /// 父会话的运行参数
    pub cfg: Arc<wb_kernel::AgentConfig>,
}

/// 服务容器：组合根（wb-api::Startup）构造一次，全部 handler 共享。
pub struct Services {
    /// 数据目录
    pub data_dir: PathBuf,
    /// 当前工作区
    pub workspace: std::sync::Mutex<String>,
    /// 持久层
    pub repo: Arc<wb_repo::Repo>,
    /// provider 配置存储
    pub providers: wb_provider::config::ProviderStore,
    /// 工具注册表（内置 + 功能提供；开关查询走 repo）
    pub registry: std::sync::RwLock<wb_tools::ToolRegistry>,
    /// 技能注册表
    pub skills: std::sync::RwLock<wb_skills::SkillRegistry>,
    /// MCP 连接池（工具并入注册表在 mcp 模块完成）
    pub mcp: Arc<wb_mcp::Pool>,
    /// 内核提问回答板（ask_user 功能与 chat/answer 路由的桥）
    pub asks: Arc<wb_features::ask_user::AnswerBoard>,
    /// 对话运行态（每会话一个 run 句柄或空闲）
    pub chat: std::sync::Mutex<chat::ChatBoard>,
    /// 事件出口
    pub emitter: std::sync::Mutex<Option<Arc<dyn Emitter>>>,
    /// 当前运行上下文：会话 id → 该会话的运行参数（多会话可同时跑）。
    /// 按会话分桶而不是单槽：并发时单槽会被后开的一轮覆盖，task 工具会拿错父参数。
    pub scope: std::sync::Mutex<std::collections::HashMap<String, RunScope>>,
    /// 目标续跑计数：会话 id → 连续续跑次数（防无限循环）
    pub goal_resumes: std::sync::Mutex<std::collections::HashMap<String, i64>>,
    /// 待办存储：压缩回填与 todo 工具共用同一份盘上状态
    pub todos: std::sync::Mutex<Option<Arc<wb_features::todo::TodoStore>>>,
    /// 活跃子代理表（task 工具写入，界面按它显示「谁在跑」）
    pub agents: Arc<agents::AgentBoard>,
    /// 内置运行时解压进度（后台线程写；/runtime 与工具提示读）
    pub runtime: runtime::RuntimeState,
    /// 会话级信任名单：用户点「本会话都批」后，这个会话里同类工具不再问
    pub trusted_tools: Arc<std::sync::Mutex<std::collections::HashMap<String, Vec<String>>>>,
}

impl Services {
    /// 装配：数据目录 + 工作区。内核客户端等重物在首次对话时构建。
    pub fn new(data_dir: PathBuf, workspace: String, repo: Arc<wb_repo::Repo>) -> Arc<Self> {
        // 运行时后台解压：启动不等 186MB 落地，解压期间工具的报错文案会指明原因
        let runtime_state = runtime::empty_state();
        let providers = wb_provider::config::ProviderStore::new(&data_dir);
        let data_dir_for_asks = data_dir.clone();
        let mut registry = wb_tools::ToolRegistry::new();
        wb_tools::builtin::install(&mut registry);
        let skills = wb_skills::SkillRegistry::scan(
            &data_dir.join("skills").join("builtin"),
            &data_dir.join("skills").join("user"),
            &PathBuf::from(&workspace).join(".workbaby").join("skills"),
        );
        let sv = Arc::new(Self {
            data_dir,
            workspace: std::sync::Mutex::new(workspace),
            repo,
            providers,
            registry: std::sync::RwLock::new(registry),
            skills: std::sync::RwLock::new(skills),
            mcp: wb_mcp::Pool::new(),
            asks: wb_features::ask_user::AnswerBoard::with_store(
                // 待答问题跟设置库放一起：重启后还能把它们重新弹给用户
                data_dir_for_asks.join("pending_asks.json"),
            ),
            chat: std::sync::Mutex::new(chat::ChatBoard::new()),
            emitter: std::sync::Mutex::new(None),
            scope: std::sync::Mutex::new(std::collections::HashMap::new()),
            goal_resumes: std::sync::Mutex::new(std::collections::HashMap::new()),
            todos: std::sync::Mutex::new(None),
            agents: agents::AgentBoard::new(),
            runtime: runtime_state,
            trusted_tools: Arc::new(std::sync::Mutex::new(std::collections::HashMap::new())),
        });
        // 待办存储与技能表共用同一份工作区解析：先装上，工具与压缩回填才有源
        sv.reload_workspace_scoped();
        sv
    }

    /// 后台解压内置运行时；进度由前端轮询 `/runtime` 读取，不另开事件通道。
    pub fn spawn_runtime(&self) {
        runtime::start_background(
            self.data_dir.clone(),
            self.runtime.clone(),
            Arc::new(|p: &runtime::RuntimeProgress| {
                if p.finished {
                    tracing::info!(
                        done = p.done_count,
                        failures = p.failures.len(),
                        "内置运行时解压结束"
                    );
                }
            }),
        );
    }

    /// 挂事件出口（Startup 时调用）。
    pub fn set_emitter(&self, emitter: Arc<dyn Emitter>) {
        *self.emitter.lock().unwrap() = Some(emitter);
    }

    /// 向全局通道推一条事件；未挂出口时静默跳过。
    pub fn emit_event(&self, session_id: &str, event: &str, data: serde_json::Value) {
        let guard = self.emitter.lock().unwrap();
        if let Some(e) = guard.as_ref() {
            e.emit(session_id, event, data);
        }
    }

    /// 当前会话目录（按工作区编码）。
    pub fn sessions_dir(&self) -> PathBuf {
        let ws = self.workspace_text();
        let encoded = ws.replace([char::from(92), '/', ':'], "-");
        self.data_dir.join("sessions").join(encoded)
    }
}

/// 建一条 mpsc 通道给内核转向队列。
pub fn steer_channel() -> (mpsc::Sender<String>, mpsc::Receiver<String>) {
    mpsc::channel(16)
}

/// 组装一轮 run 的公共参数（工具开关 + 技能段 + 项目上下文 + 提示词）。
/// `session_id` 决定活状态里哪些内容属于本会话（目标按会话隔离）。
pub fn inputs(sv: &Arc<Services>, session_id: &str) -> (Vec<String>, String) {
    let (direct, _deferred) = tool_sets(sv);
    // 直接进请求的工具里，只有开着的才算；延迟的那批此刻还没加载
    let enabled_tools: Vec<String> = direct
        .iter()
        .filter(|t| sv.repo.tool_enabled(t).unwrap_or(true))
        .cloned()
        .collect();
    let skills = sv.skills.read().unwrap();
    let state = features::live_state(sv, session_id);
    let context = project_context_text(sv);
    let prompt = prompt::build_system_prompt(
        &sv.workspace_text(),
        &skills,
        &enabled_tools,
        state.as_deref(),
        &context,
        |name| sv.repo.skill_enabled(name).unwrap_or(true),
    );
    (enabled_tools, prompt)
}

/// 工具分成两批：直接进请求的与延迟加载的。
///
/// 延迟加载的那批只在模型用 tool_search 搜到后才进请求。默认关——
/// 延迟加载会让模型先搜一轮，多一轮往返；工具不多时这笔买卖不划算。
pub fn tool_sets(sv: &Arc<Services>) -> (Vec<String>, Vec<String>) {
    let all: Vec<String> = sv
        .registry
        .read()
        .unwrap()
        .all()
        .iter()
        .map(|t| t.name().to_string())
        .filter(|n| sv.repo.tool_enabled(n).unwrap_or(true))
        .collect();
    if !defer_enabled(sv) {
        // 不开延迟加载时 tool_search 不进请求：它在注册表里只是为了能随时启用
        let direct = all
            .iter()
            .filter(|n| n.as_str() != "tool_search")
            .cloned()
            .collect();
        return (direct, Vec::new());
    }
    // tool_search 是入口，必须一直在请求里；其余全部延迟
    let direct = vec!["tool_search".to_string()];
    let deferred = all.into_iter().filter(|n| n != "tool_search").collect();
    (direct, deferred)
}

/// 是否开工具延迟加载（设置项 `defer_tools`）。
pub fn defer_enabled(sv: &Arc<Services>) -> bool {
    sv.repo
        .settings_get("defer_tools")
        .ok()
        .flatten()
        .map(|v| v == "true")
        .unwrap_or(false)
}

/// 每轮都重发的固定开销：系统提示词 + 工具定义。
/// 它们不在回放消息里，但不计入就会高估剩余窗口——压缩该发生时没发生。
pub fn context_overhead(
    sv: &Arc<Services>,
    session_id: &str,
) -> wb_kernel::compaction::ContextOverhead {
    let (enabled_tools, prompt) = inputs(sv, session_id);
    let registry = sv.registry.read().unwrap();
    let tool_tokens: i64 = enabled_tools
        .iter()
        .filter_map(|n| registry.get(n))
        .map(|t| {
            wb_kernel::tokenest::estimate_text_tokens(t.description())
                + wb_kernel::tokenest::estimate_text_tokens(&t.parameters().to_string())
        })
        .sum();
    wb_kernel::compaction::ContextOverhead {
        system_tokens: wb_kernel::tokenest::estimate_text_tokens(&prompt),
        tool_tokens,
    }
}

/// 项目上下文段：设置项 `context_files` 关掉时返回空串（用户可能不想让工作区里的
/// 文件影响模型行为）。发现失败按无上下文处理——读不到不该拦住整轮对话。
pub fn project_context_text(sv: &Arc<Services>) -> String {
    let on = sv
        .repo
        .settings_get("context_files")
        .ok()
        .flatten()
        .map(|v| v != "off")
        .unwrap_or(true);
    if !on {
        return String::new();
    }
    let files = wb_skills::discover(&sv.workspace_dir());
    wb_skills::render(&files)
}
