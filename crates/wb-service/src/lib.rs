//! 业务编排层：对话 run 管理、KernelEvent→SSE 翻译、系统提示词组装、目录与装配。
//! 依赖全部能力域，但不引 axum / tauri。

pub mod chat;
pub mod chat_events;
pub mod context;
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
pub mod store;
pub mod task;

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
    /// 当前运行上下文（task 工具派子代理时取父会话参数）
    pub scope: std::sync::Mutex<Option<RunScope>>,
    /// 目标续跑计数：会话 id → 连续续跑次数（防无限循环）
    pub goal_resumes: std::sync::Mutex<std::collections::HashMap<String, i64>>,
}

impl Services {
    /// 装配：数据目录 + 工作区。内核客户端等重物在首次对话时构建。
    pub fn new(data_dir: PathBuf, workspace: String, repo: Arc<wb_repo::Repo>) -> Arc<Self> {
        let providers = wb_provider::config::ProviderStore::new(&data_dir);
        let mut registry = wb_tools::ToolRegistry::new();
        wb_tools::builtin::install(&mut registry);
        let skills = wb_skills::SkillRegistry::scan(
            &data_dir.join("skills").join("builtin"),
            &data_dir.join("skills").join("user"),
            &PathBuf::from(&workspace).join(".workbaby").join("skills"),
        );
        Arc::new(Self {
            data_dir,
            workspace: std::sync::Mutex::new(workspace),
            repo,
            providers,
            registry: std::sync::RwLock::new(registry),
            skills: std::sync::RwLock::new(skills),
            mcp: wb_mcp::Pool::new(),
            asks: wb_features::ask_user::AnswerBoard::new(),
            chat: std::sync::Mutex::new(chat::ChatBoard::new()),
            emitter: std::sync::Mutex::new(None),
            scope: std::sync::Mutex::new(None),
            goal_resumes: std::sync::Mutex::new(std::collections::HashMap::new()),
        })
    }

    /// 挂事件出口（Startup 时调用）。
    pub fn set_emitter(&self, emitter: Arc<dyn Emitter>) {
        *self.emitter.lock().unwrap() = Some(emitter);
    }

    /// 当前会话目录（按工作区编码）。
    pub fn sessions_dir(&self) -> PathBuf {
        let ws = self.workspace.lock().unwrap().clone();
        let encoded = ws.replace([char::from(92), '/', ':'], "-");
        self.data_dir.join("sessions").join(encoded)
    }
}

/// 建一条 mpsc 通道给内核转向队列。
pub fn steer_channel() -> (mpsc::Sender<String>, mpsc::Receiver<String>) {
    mpsc::channel(16)
}

/// 组装一轮 run 的公共参数（工具开关 + 技能段 + 提示词）。
pub fn inputs(sv: &Arc<Services>) -> (Vec<String>, String) {
    let enabled_tools: Vec<String> = sv
        .registry
        .read()
        .unwrap()
        .all()
        .iter()
        .filter(|t| sv.repo.tool_enabled(t.name()).unwrap_or(true))
        .map(|t| t.name().to_string())
        .collect();
    let skills = sv.skills.read().unwrap();
    let prompt = prompt::build_system_prompt(
        &sv.workspace.lock().unwrap().clone(),
        &skills,
        &enabled_tools,
    );
    (enabled_tools, prompt)
}
