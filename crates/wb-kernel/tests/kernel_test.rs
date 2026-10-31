//! wb-kernel 长链路测试：内核循环、会话树持久化、子代理。
//! 脚本化假 Provider 回放事件流；断言集中在事件顺序契约、落盘回放与上下文隔离。

use std::path::Path;
use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use futures::stream;
use tokio_util::sync::CancellationToken;
use wb_kernel::{AgentConfig, KernelEvent, SubagentConfig};
use wb_provider::{
    ContentPart, ModelClient, ModelEvent, ProviderStream, ReqMessage, Role, StopReason,
    StreamRequest, ThinkingLevel, ToolCallDone, Usage,
};
use wb_tools::{Tool, ToolOutput, ToolRegistry};

// 事件脚本构造器：一个场景一轮脚本，三五行说清。
fn delta(t: &str) -> ModelEvent {
    ModelEvent::TextDelta(0, t.into())
}

fn tool_end(idx: usize, id: &str, args: &str) -> ModelEvent {
    ModelEvent::ToolCallEnd(
        idx,
        ToolCallDone {
            call_id: id.into(),
            name: "echo".into(),
            arguments: args.into(),
        },
    )
}

fn done(input: i64, stop: StopReason) -> ModelEvent {
    ModelEvent::Done(
        Usage {
            input,
            ..Default::default()
        },
        stop,
    )
}

fn tool_use() -> ModelEvent {
    done(0, StopReason::ToolUse)
}

/// 脚本化假 Provider：stream() 弹出一段预设事件流，并记录收到的请求。
struct FakeProvider {
    turns: Mutex<Vec<Vec<ModelEvent>>>,
    seen: Mutex<Vec<StreamRequest>>,
}

impl FakeProvider {
    fn new(turns: Vec<Vec<ModelEvent>>) -> Arc<Self> {
        Arc::new(Self {
            turns: Mutex::new(turns),
            seen: Mutex::new(Vec::new()),
        })
    }

    /// 首个请求里系统消息的正文拼接。
    fn first_system_text(&self) -> String {
        let seen = self.seen.lock().unwrap();
        let parts = match &seen[0].messages[0] {
            ReqMessage {
                role: Role::System,
                parts,
                ..
            } => parts,
            _ => panic!("首条应是系统消息"),
        };
        parts
            .iter()
            .map(|p| match p {
                ContentPart::Text(t) => t.clone(),
                _ => String::new(),
            })
            .collect()
    }
}

#[async_trait]
impl ModelClient for FakeProvider {
    async fn stream(
        &self,
        req: StreamRequest,
        _cancel: CancellationToken,
    ) -> wb_pkg::Result<ProviderStream> {
        self.seen.lock().unwrap().push(req);
        let next = self.turns.lock().unwrap().pop();
        let script = next.unwrap_or_else(|| vec![done(0, StopReason::Stop)]);
        Ok(Box::pin(stream::iter(script)))
    }
}

/// 回声工具：原样返回输入。
struct EchoTool;

#[async_trait]
impl Tool for EchoTool {
    fn name(&self) -> &str {
        "echo"
    }
    fn label(&self) -> &str {
        "回声"
    }
    fn description(&self) -> &str {
        "原样返回输入文本"
    }
    fn parameters(&self) -> serde_json::Value {
        serde_json::json!({"type":"object","properties":{"text":{"type":"string"}},"required":["text"]})
    }
    async fn execute(
        &self,
        args: serde_json::Value,
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> wb_pkg::Result<ToolOutput> {
        Ok(ToolOutput {
            text: args
                .get("text")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .into(),
            is_error: false,
            duration_ms: 1,
        })
    }
}

/// 事件名序列（断言顺序契约用）。
fn event_name(ev: &KernelEvent) -> &'static str {
    match ev {
        KernelEvent::AgentStart { .. } => "agent_start",
        KernelEvent::TurnStart => "turn_start",
        KernelEvent::MessageEnd { .. } => "message_end",
        KernelEvent::TextDelta { .. } => "text_delta",
        KernelEvent::ThinkingDelta { .. } => "thinking_delta",
        KernelEvent::ToolCallDelta { .. } => "tool_call_delta",
        KernelEvent::ToolExecutionStart { .. } => "tool_start",
        KernelEvent::ToolExecutionEnd { .. } => "tool_end",
        KernelEvent::TurnEnd { .. } => "turn_end",
        KernelEvent::QueueUpdate { .. } => "queue_update",
        KernelEvent::CompactionStart { .. } => "compaction_start",
        KernelEvent::CompactionEnd { .. } => "compaction_end",
        KernelEvent::AutoRetry { .. } => "retry",
        KernelEvent::AssistantFinal { .. } => "assistant_final",
        KernelEvent::AgentEnd { .. } => "agent_end",
        KernelEvent::Settled { .. } => "settled",
        KernelEvent::Failed { .. } => "failed",
    }
}

/// 临时目录（带唯一后缀，避免并行用例互相踩）。
fn temp_dir(tag: &str) -> std::path::PathBuf {
    let dir = std::env::temp_dir().join(format!("wb_kernel_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    dir
}

/// 造一个只带 echo 工具的内核配置。
fn agent_cfg(provider: Arc<FakeProvider>) -> AgentConfig {
    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(EchoTool));
    AgentConfig {
        client: provider,
        registry: Arc::new(registry),
        enabled_tools: vec!["echo".into()],
        system_prompt: "你是测试助手".into(),
        provider_id: "test".into(),
        model_id: "fake-model".into(),
        context_window: 100_000,
        max_tokens: 1024,
        thinking: ThinkingLevel::Off,
        max_retries: 0,
        workdir: temp_dir("cfg"),
    }
}

/// 加载目录里唯一的 jsonl 会话文件。
fn load_session(dir: &Path) -> wb_kernel::SessionFile {
    let file = std::fs::read_dir(dir)
        .unwrap()
        .flatten()
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "jsonl"))
        .expect("应有会话文件");
    wb_kernel::SessionFile::load(&file).expect("会话文件应可加载")
}

/// 回放消息的角色序。
fn role_names(msgs: &[wb_kernel::Msg]) -> Vec<&'static str> {
    msgs.iter()
        .map(|m| match m {
            wb_kernel::Msg::User { .. } => "user",
            wb_kernel::Msg::Assistant { .. } => "assistant",
            wb_kernel::Msg::ToolResult { .. } => "tool",
        })
        .collect()
}

/// 子代理配置（默认探底参数，tools 覆盖工具清单）。
fn sub_cfg(tmp: &Path, tools: &[&str]) -> SubagentConfig {
    SubagentConfig {
        parent_session_id: "SESS_parent".into(),
        role_prompt: String::new(),
        task: "干活".into(),
        tool_names: tools.iter().map(|t| t.to_string()).collect(),
        sessions_dir: tmp.to_path_buf(),
        workspace: "D:/ws".into(),
        max_turns: 8,
    }
}

/// 内核循环全链路：单工具批的事件顺序契约 + 并行工具批一个不能少。
#[tokio::test]
async fn agent_loop_replays_tool_batches_end_to_end() {
    // 阶段一：两轮采样夹一次工具调用；事件顺序、落盘回放、回灌请求三头验。
    let tmp = temp_dir("run");
    let provider = FakeProvider::new(vec![
        // 第二轮：收尾
        vec![delta("工具结果已确认"), done(100, StopReason::Stop)],
        // 第一轮：正文 + 工具调用
        vec![
            delta("我先调用工具"),
            tool_end(1, "call_1", r#"{"text":"你好"}"#),
            tool_use(),
        ],
    ]);

    let events: Arc<Mutex<Vec<&'static str>>> = Arc::new(Mutex::new(Vec::new()));
    let sink = events.clone();
    let emit: wb_kernel::EmitFn =
        Arc::new(move |ev: &KernelEvent| sink.lock().unwrap().push(event_name(ev)));
    let (_steer_tx, steer_rx) = tokio::sync::mpsc::channel(4);

    let outcome = wb_kernel::run(
        wb_kernel::SessionFile::create(&tmp, "D:/ws", "SESS_run"),
        agent_cfg(provider.clone()),
        emit,
        CancellationToken::new(),
        steer_rx,
        "请调用 echo 工具".into(),
    )
    .await
    .expect("一轮运行不应失败");
    assert!(!outcome.aborted, "不该被中止");
    assert_eq!(outcome.last_text, "工具结果已确认", "最后一轮正文应回传");

    // 事件顺序契约：final 先于 message_end，工具批夹在两 turn 之间，settled 收尾
    let expect =
        "agent_start turn_start text_delta assistant_final message_end tool_start tool_end \
        turn_end turn_start text_delta assistant_final message_end turn_end agent_end settled";
    assert_eq!(
        events.lock().unwrap().join(" "),
        expect,
        "事件顺序契约被破坏：界面时序依赖它"
    );

    // 落盘回放：user → assistant(工具调用) → tool(结果) → assistant(正文)
    let msgs = load_session(&tmp).replay_messages();
    assert_eq!(
        role_names(&msgs),
        ["user", "assistant", "tool", "assistant"],
        "回放序错乱"
    );
    let wb_kernel::Msg::ToolResult { content, .. } = &msgs[2] else {
        panic!("第三条应是工具结果");
    };
    assert_eq!(content, "你好", "工具结果应原样回灌");
    // 第二轮请求应把工具结果回灌（消息数：system+user+assistant+tool = 4）
    assert_eq!(
        provider.seen.lock().unwrap()[1].messages.len(),
        4,
        "工具结果必须紧跟其调用回灌"
    );
    let _ = std::fs::remove_dir_all(&tmp);

    // 阶段二：并行工具批——漏掉一个就是丢掉一次调用，模型以为做过了实际没做。
    let tmp = temp_dir("parallel");
    let provider = FakeProvider::new(vec![
        vec![delta("都查完了"), done(0, StopReason::Stop)],
        vec![
            tool_end(0, "c0", r#"{"text":"A"}"#),
            tool_end(1, "c1", r#"{"text":"B"}"#),
            tool_use(),
        ],
    ]);
    let started = Arc::new(Mutex::new(Vec::<String>::new()));
    let sink = started.clone();
    let emit: wb_kernel::EmitFn = Arc::new(move |ev: &KernelEvent| {
        if let KernelEvent::ToolExecutionStart { args, .. } = ev {
            let text = args["text"].as_str().unwrap_or("").to_string();
            sink.lock().unwrap().push(text);
        }
    });
    let (_tx, rx) = tokio::sync::mpsc::channel(4);
    wb_kernel::run(
        wb_kernel::SessionFile::create(&tmp, "D:/ws", "SESS_par"),
        agent_cfg(provider),
        emit,
        CancellationToken::new(),
        rx,
        "并行查两个".into(),
    )
    .await
    .expect("不应失败");
    let mut seen = started.lock().unwrap().clone();
    seen.sort();
    assert_eq!(seen, ["A", "B"], "两个调用都要执行");
    let results: Vec<String> = load_session(&tmp)
        .replay_messages()
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::ToolResult { content, .. } => Some(content.clone()),
            _ => None,
        })
        .collect();
    assert_eq!(results.len(), 2, "两个工具结果都要落盘，实际 {results:?}");
    let _ = std::fs::remove_dir_all(&tmp);
}

/// 会话文件语义：追加 → 重载 → 回放一字不差；压缩以摘要替代早期历史；
/// ContextEdit 把目标条目从回放里摘掉。压缩与回溯都建立在它上面。
#[test]
fn session_file_survives_reload_compaction_and_context_edit() {
    let dir = temp_dir("session");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_TEST");
    let user = |text: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: text.into(),
            timestamp: ts,
        },
    };
    sf.append(user("hi", 1)).unwrap();
    sf.append(wb_kernel::Entry::Message {
        message: wb_kernel::Msg::Assistant {
            content: vec![wb_kernel::Block::Text {
                text: "hello".into(),
            }],
            provider_id: "p".into(),
            model_id: "m".into(),
            message_id: "MSG_1".into(),
            usage: None,
            stop_reason: "stop".into(),
            timestamp: 2,
            duration_ms: 5,
        },
    })
    .unwrap();

    let path = sf.path.clone();
    drop(sf);
    let mut reloaded = wb_kernel::SessionFile::load(&path).unwrap();
    assert_eq!(reloaded.replay_messages().len(), 2, "重载后消息数不该变");

    // 压缩记录落在树上：回放时早期消息被摘要替代
    reloaded
        .append(wb_kernel::Entry::Compaction {
            summary: "之前聊了问候".into(),
            first_kept_entry_id: String::new(),
            tokens_before: 100,
        })
        .unwrap();
    let after = reloaded.replay_messages();
    assert_eq!(after.len(), 1, "压缩后只剩摘要一条");
    match &after[0] {
        wb_kernel::Msg::User { content, .. } => {
            assert!(
                content.contains("之前聊了问候"),
                "摘要应进入上下文：{content}"
            )
        }
        other => panic!("压缩后应是摘要形态的 user 消息，得到 {other:?}"),
    }

    // ContextEdit：目标条目被摘出回放
    let bad = reloaded.append(user("bad", 3)).unwrap().entry_id.clone();
    reloaded
        .append(wb_kernel::Entry::ContextEdit {
            target_entry_id: bad,
            replacement: None,
        })
        .unwrap();
    assert_eq!(
        reloaded.replay_messages().len(),
        1,
        "被编辑的条目不应出现在回放里"
    );

    let _ = std::fs::remove_dir_all(&dir);
}

/// 子代理跑完只把结论带回来，中间过程留在子会话文件里；
/// 提示词继承角色与工作区段，但不带父提示词正文。
#[tokio::test]
async fn subagent_isolates_context_and_returns_conclusion() {
    let tmp = temp_dir("subagent");
    let provider = FakeProvider::new(vec![
        // 第二轮：收尾结论
        vec![delta("共找到 3 个匹配文件"), done(30, StopReason::Stop)],
        // 第一轮：调工具
        vec![
            tool_end(1, "c1", r#"{"text":"扫描结果"}"#),
            done(30, StopReason::ToolUse),
        ],
    ]);

    let mut cfg = agent_cfg(provider.clone());
    cfg.system_prompt = "父助手提示词\n<cwd>D:/ws</cwd>\n<skills>\n- demo\n</skills>".into();
    let mut sc = sub_cfg(&tmp, &["echo"]);
    sc.role_prompt = "你是探查员".into();
    sc.task = "扫描工作区并汇总".into();

    let outcome = wb_kernel::run_subagent(&cfg, sc, CancellationToken::new())
        .await
        .expect("子代理不应失败");
    assert_eq!(outcome.conclusion, "共找到 3 个匹配文件", "结论应原样带回");
    assert!(
        outcome.session_id.starts_with("SUB_"),
        "会话 id 应带 SUB_ 前缀"
    );
    assert_eq!(outcome.usage.input, 60, "两轮用量应累加");

    // 隔离：子会话文件独立落盘，父上下文看不到中间过程
    let msgs = load_session(&tmp).replay_messages();
    assert_eq!(
        role_names(&msgs),
        ["user", "assistant", "tool", "assistant"]
    );
    let wb_kernel::Msg::ToolResult { content, .. } = &msgs[2] else {
        panic!("第三条应是工具结果");
    };
    assert_eq!(content, "扫描结果", "工具结果应留在子会话里");

    // 子提示词带角色与工作区段，不带父提示词正文
    let sys = provider.first_system_text();
    assert!(sys.contains("你是探查员"), "子提示词应含角色职责");
    assert!(sys.contains("D:/ws"), "子提示词应继承工作区");
    assert!(
        !sys.contains("父助手提示词"),
        "不应把父提示词正文整段灌进子代理"
    );

    let _ = std::fs::remove_dir_all(&tmp);
}

/// 工具白名单收窄：子代理拿不到父会话之外的工具，也不能再派子代理。
#[tokio::test]
async fn subagent_tool_list_is_narrowed() {
    let tmp = temp_dir("subagent-tools");
    let provider = FakeProvider::new(vec![vec![delta("结论"), done(0, StopReason::Stop)]]);
    let mut cfg = agent_cfg(provider.clone());
    cfg.enabled_tools = vec!["echo".into(), "task".into()];
    let mut sc = sub_cfg(&tmp, &["echo"]);
    sc.tool_names = vec!["echo".into(), "task".into()];

    wb_kernel::run_subagent(&cfg, sc, CancellationToken::new())
        .await
        .expect("子代理不应失败");

    let names: Vec<String> = provider.seen.lock().unwrap()[0]
        .tools
        .iter()
        .map(|t| t.name.clone())
        .collect();
    assert_eq!(names, ["echo"], "task 应被剔除，避免子代理再派子代理");

    let _ = std::fs::remove_dir_all(&tmp);
}

/// 轮次用尽时回退到子代理最后说的正文，不给父会话一句空话。
#[tokio::test]
async fn subagent_falls_back_to_last_text_when_turn_limit_hit() {
    let tmp = temp_dir("subagent-limit");
    let provider = FakeProvider::new(vec![
        // 第二轮：只调工具不说话
        vec![tool_end(2, "c2", r#"{"text":"y"}"#), tool_use()],
        // 第一轮：说话 + 调工具，永远不收尾——轮次上限是唯一的出口
        vec![
            delta("我先看看文件"),
            tool_end(1, "c1", r#"{"text":"x"}"#),
            tool_use(),
        ],
    ]);

    let mut sc = sub_cfg(&tmp, &["echo"]);
    sc.max_turns = 2;
    let outcome = wb_kernel::run_subagent(&agent_cfg(provider), sc, CancellationToken::new())
        .await
        .expect("子代理不应报错");
    assert!(
        outcome.conclusion.contains("我先看看文件"),
        "轮次用尽应回退到最后一段正文，实际：{}",
        outcome.conclusion
    );

    let _ = std::fs::remove_dir_all(&tmp);
}
