//! 功能装配：FeatureCtx 构造 + 统一注册表组装（内置 + 功能工具 + MCP 桥）。

use std::sync::Arc;

use super::Services;

/// 功能上下文：从 Services 借视图（wb-features 不反向依赖 service）。
fn feature_ctx(sv: &Arc<Services>) -> wb_features::FeatureCtx {
    let emitter = sv.emitter.lock().unwrap().clone();
    wb_features::FeatureCtx {
        data_dir: sv.data_dir.clone(),
        workspace: sv.workspace.lock().unwrap().clone().into(),
        emit: Arc::new(move |sid, event, data| {
            if let Some(e) = &emitter {
                e.emit(sid, event, data);
            }
        }),
    }
}

/// 功能是否启用（repo 开关；缺省按 docs/FEATURES.md 的默认值）。
fn enabled(sv: &Arc<Services>, code: &str, default_on: bool) -> bool {
    sv.repo
        .feature_enabled(code)
        .ok()
        .flatten()
        .unwrap_or(default_on)
}

/// 目标续跑提示：功能开启且存在活跃目标时给下一步提示，否则 None。
///
/// 目标功能关掉时不续跑——旧的目标文件不该在功能关闭后还驱动对话。
pub fn goal_continuation(sv: &Arc<Services>) -> Option<String> {
    if !enabled(sv, "goal", false) {
        return None;
    }
    wb_features::goal::continuation_prompt(&feature_ctx(sv))
}

/// 组装完整注册表：内置工具打底，功能工具按开关并入，MCP 桥最后（同名可覆盖）。
pub fn build_registry(sv: &Arc<Services>) -> wb_tools::ToolRegistry {
    let mut reg = wb_tools::ToolRegistry::new();
    wb_tools::builtin::install(&mut reg);
    // task 工具常驻：它是把大块活摘出主上下文的手段，关掉就等于没有子代理。
    reg.register(Arc::new(super::task::TaskTool::new(sv.clone())));
    let ctx = feature_ctx(sv);
    if enabled(sv, "ask_user", true) {
        reg.register(Arc::new(wb_features::ask_user::AskUserTool::new(
            ctx.clone(),
            sv.asks.clone(),
        )));
    }
    if enabled(sv, "todo", true) {
        let store = Arc::new(wb_features::todo::TodoStore::new(&ctx));
        reg.register(Arc::new(wb_features::todo::TodoTool::new(
            store,
            ctx.clone(),
        )));
    }
    if enabled(sv, "fff", true) {
        match wb_features::fff::FrecencyStore::new(&ctx) {
            Ok(store) => {
                let store = Arc::new(store);
                reg.register(Arc::new(wb_features::fff::FffindTool::new(
                    Arc::clone(&store),
                    ctx.clone(),
                )));
                reg.register(Arc::new(wb_features::fff::FfgrepTool::new(
                    store,
                    ctx.clone(),
                )));
            }
            Err(e) => tracing::warn!("fff 初始化失败：{}", e.message),
        }
    }
    if enabled(sv, "memory", false) {
        match wb_features::memory_store::MemoryStore::open(&ctx) {
            Ok(store) => {
                let store = Arc::new(store);
                reg.register(Arc::new(wb_features::memory::MemorySaveTool::new(
                    store.clone(),
                )));
                reg.register(Arc::new(wb_features::memory::MemorySearchTool::new(
                    store.clone(),
                )));
                reg.register(Arc::new(wb_features::memory::MemoryRemoveTool::new(
                    store.clone(),
                )));
                reg.register(Arc::new(wb_features::memory::MemoryConsolidateTool::new(
                    store.clone(),
                )));
                reg.register(Arc::new(wb_features::memory::SessionSearchTool::new(
                    ctx.clone(),
                )));
            }
            Err(e) => tracing::warn!("memory 初始化失败：{}", e.message),
        }
    }
    if enabled(sv, "web", false) {
        reg.register(Arc::new(wb_features::web::FetchTool::new()));
        reg.register(Arc::new(wb_features::web::SearchTool::new()));
        reg.register(Arc::new(wb_features::web_files::PdfReadTool::new()));
        reg.register(Arc::new(wb_features::web_files::GithubCloneTool));
        reg.register(Arc::new(wb_features::web_video::VideoTool::new()));
    }
    if enabled(sv, "goal", false) {
        let store = Arc::new(wb_features::goal::GoalStore::new(&ctx));
        reg.register(Arc::new(wb_features::goal::CreateGoalTool::new(
            store.clone(),
            ctx.clone(),
        )));
        reg.register(Arc::new(wb_features::goal::GetGoalTool::new(store.clone())));
        reg.register(Arc::new(wb_features::goal::UpdateGoalTool::new(
            store,
            ctx.clone(),
        )));
    }
    if enabled(sv, "plan", false) {
        let store = Arc::new(wb_features::plan_types::PlanStore::new(&ctx));
        reg.register(Arc::new(wb_features::plan::PlanSubmitTool::new(
            store.clone(),
            ctx.clone(),
            sv.asks.clone(),
        )));
        reg.register(Arc::new(wb_features::plan::PlanMarkDoneTool::new(
            store.clone(),
            ctx.clone(),
        )));
        reg.register(Arc::new(wb_features::plan::PlanInboxTool::new(
            store.clone(),
        )));
        reg.register(Arc::new(wb_features::plan_annotate::PlanAnnotateTool::new(
            store,
            ctx.clone(),
        )));
        // PR 审查：本地分支差异就是 PR 的本地等价物，不需连远端。
        reg.register(Arc::new(super::review::ReviewDiffTool::new(sv.clone())));
    }
    if enabled(sv, "context_mode", false) {
        reg.register(Arc::new(wb_features::context_mode::CtxExecuteTool::new(
            ctx.clone(),
        )));
        reg.register(Arc::new(
            wb_features::context_mode::CtxBatchExecuteTool::new(ctx.clone()),
        ));
        reg.register(Arc::new(wb_features::context_mode::CtxIndexTool::new(
            ctx.clone(),
        )));
        reg.register(Arc::new(wb_features::context_mode::CtxSearchTool::new(
            ctx.clone(),
        )));
    }
    if enabled(sv, "lsp", false) {
        reg.register(Arc::new(
            wb_features::lsp::diagnostics::LspDiagnosticsTool::new(ctx.clone()),
        ));
        reg.register(Arc::new(wb_features::lsp::fix::LspFixTool::new(
            ctx.clone(),
        )));
    }
    super::mcp::install_mcp_tools(sv, &mut reg);
    reg
}

/// 重建并写回注册表（开关变化 / MCP reload 后调用）。
pub fn refresh_registry(sv: &Arc<Services>) {
    *sv.registry.write().unwrap() = build_registry(sv);
}

/// 12 项内置功能的静态清单（默认值见 docs/FEATURES.md）。
pub fn list_vo(sv: &Arc<Services>) -> Vec<wb_domain::feature::FeatureVO> {
    const FEATURES: [(&str, &str, &str, bool, &str); 12] = [
        (
            "todo",
            "待办清单",
            "实时待办面板，跨压缩与重启存活",
            true,
            "p1",
        ),
        (
            "ask_user",
            "结构化提问",
            "让模型在该猜时改为向你提问",
            true,
            "p1",
        ),
        (
            "fff",
            "模糊搜索",
            "frecency 排序的模糊文件与内容搜索",
            true,
            "p1",
        ),
        (
            "context_view",
            "上下文地图",
            "上下文占用的分解视图",
            true,
            "p1",
        ),
        (
            "memory",
            "持久记忆",
            "SQLite FTS5 记忆与历史会话检索",
            false,
            "p1",
        ),
        ("web", "联网访问", "搜索 / 抓取 / 来源核查", false, "p1"),
        (
            "loadout",
            "装载配置",
            "工具与技能开关的命名预设",
            false,
            "p1",
        ),
        (
            "context_mode",
            "省上下文",
            "沙箱执行 + FTS5 知识库",
            false,
            "p2",
        ),
        (
            "lsp",
            "语言服务",
            "诊断与修复（需自装语言服务器）",
            false,
            "p2",
        ),
        ("plan", "计划评审", "交互式计划标注与审查", false, "p2"),
        ("goal", "目标追踪", "目标状态机与自动续跑", false, "p2"),
        (
            "inspect",
            "内省面板",
            "工具/命令/技能/提示词快照",
            false,
            "p2",
        ),
    ];
    FEATURES
        .iter()
        .map(
            |(code, name, desc, default_on, phase)| wb_domain::feature::FeatureVO {
                feature_code: code.to_string(),
                feature_name: name.to_string(),
                feature_description: desc.to_string(),
                feature_enabled: enabled(sv, code, *default_on),
                feature_configurable: matches!(*code, "web" | "memory" | "context_mode"),
                // 已落地的功能报出工具全名；其余按阶段推进后补
                feature_tools: match *code {
                    "todo" => vec!["todo".into()],
                    "ask_user" => vec!["ask_user_question".into()],
                    "fff" => vec!["fffind".into(), "ffgrep".into()],
                    "memory" => vec![
                        "memory_save".into(),
                        "memory_search".into(),
                        "memory_remove".into(),
                        "memory_consolidate".into(),
                        "session_search".into(),
                    ],
                    "web" => vec![
                        "fetch_content".into(),
                        "web_search".into(),
                        "pdf_read".into(),
                        "github_clone".into(),
                        "video_understand".into(),
                    ],
                    "context_mode" => vec![
                        "ctx_execute".into(),
                        "ctx_batch_execute".into(),
                        "ctx_index".into(),
                        "ctx_search".into(),
                    ],
                    "lsp" => vec!["lsp_diagnostics".into(), "lsp_fix".into()],
                    "goal" => vec![
                        "create_goal".into(),
                        "get_goal".into(),
                        "update_goal".into(),
                    ],
                    "plan" => vec![
                        "plan_submit".into(),
                        "plan_mark_done".into(),
                        "plan_inbox".into(),
                        "plan_annotate".into(),
                        "review_diff".into(),
                    ],
                    _ => Vec::new(),
                },
                feature_phase: phase.to_string(),
            },
        )
        .collect()
}

/// Serialize 便捷包装。
fn to_val<T: serde::Serialize>(v: &T) -> serde_json::Value {
    serde_json::to_value(v).unwrap_or_default()
}

/// 自省快照（inspect 功能数据面）：当前装配一屏看全。
pub fn inspect_snapshot(
    sv: &Arc<Services>,
    session_id: &str,
) -> wb_domain::resource::InspectSnapshotRESP {
    let tools = super::resources::tools_all_vo(sv);
    let skills = sv
        .skills
        .read()
        .unwrap()
        .list_vo(|n| sv.repo.skill_enabled(n).unwrap_or(true));
    let commands = super::misc::commands_list(sv);
    let (_, prompt) = super::inputs(sv);
    let session = if session_id.is_empty() {
        None
    } else {
        super::store::list(sv)
            .ok()
            .and_then(|all| all.into_iter().find(|s| s.session_id == session_id))
            .map(|s| to_val(&s))
    };
    wb_domain::resource::InspectSnapshotRESP {
        resp_tools: tools.iter().map(to_val).collect(),
        resp_skills: skills.iter().map(to_val).collect(),
        resp_commands: commands.iter().map(to_val).collect(),
        resp_system_prompt: prompt,
        resp_features: list_vo(sv).iter().map(to_val).collect(),
        resp_session: session,
    }
}

/// 启动装配：初始注册表已在 Services::new 建过一次，这里按功能开关重建。
pub fn install(sv: &Arc<Services>) {
    refresh_registry(sv);
}
