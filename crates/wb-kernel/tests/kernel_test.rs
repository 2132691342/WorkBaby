//! wb-kernel 长链路测试：会话树持久化 + 内核循环 + 子代理。
//! 用脚本化假 Provider 回放模型事件流，不起真网络。
//! 断言集中在「改一处会坏一片」的地方：事件顺序契约、落盘回放、上下文隔离。

use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use futures::stream;
use tokio_util::sync::CancellationToken;
use wb_kernel::{AgentConfig, KernelEvent, SubagentConfig};
use wb_provider::{
    ModelClient, ModelEvent, ProviderStream, StopReason, StreamRequest, ThinkingLevel,
    ToolCallDone, Usage,
};
use wb_tools::{Tool, ToolOutput, ToolRegistry};

/// 脚本化假 Provider：每次 stream() 弹出一段预设事件流。
struct FakeProvider {
    /// 每轮的事件脚本（依次弹出）
    turns: Mutex<Vec<Vec<ModelEvent>>>,
    /// 收到的请求（供断言工具声明与消息回灌）
    seen: Mutex<Vec<StreamRequest>>,
}

impl FakeProvider {
    fn new(turns: Vec<Vec<ModelEvent>>) -> Arc<Self> {
        Arc::new(Self {
            turns: Mutex::new(turns),
            seen: Mutex::new(Vec::new()),
        })
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
        let script = self
            .turns
            .lock()
            .unwrap()
            .pop()
            .unwrap_or_else(|| vec![ModelEvent::Done(Usage::default(), StopReason::Stop)]);
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
        serde_json::json!({
            "type": "object",
            "properties": { "text": { "type": "string" } },
            "required": ["text"]
        })
    }

    async fn execute(
        &self,
        args: serde_json::Value,
        _cancel: CancellationToken,
        _workdir: &std::path::Path,
    ) -> wb_pkg::Result<ToolOutput> {
        let text = args
            .get("text")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        Ok(ToolOutput {
            text,
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

/// 造一个只带 echo 工具的内核配置。
fn agent_cfg(provider: Arc<FakeProvider>) -> AgentConfig {
    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(EchoTool));
    AgentConfig {
        client: provider.clone(),
        registry: Arc::new(registry),
        enabled_tools: vec!["echo".into()],
        system_prompt: "你是测试助手".into(),
        provider_id: "test".into(),
        model_id: "fake-model".into(),
        context_window: 100_000,
        max_tokens: 1024,
        thinking: ThinkingLevel::Off,
        max_retries: 0,
    }
}

/// 临时目录（带唯一后缀，避免并行用例互相踩）。
fn temp_dir(tag: &str) -> std::path::PathBuf {
    let dir = std::env::temp_dir().join(format!("wb_kernel_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    dir
}

/// 全链路：两轮采样夹一次工具调用；顺序、落盘、用量三头验。
#[tokio::test]
async fn run_with_tool_call_replays_end_to_end() {
    let tmp = temp_dir("run");
    let sid = format!("SESS_{}", wb_pkg::ulid::raw_ulid());
    let session = wb_kernel::SessionFile::create(&tmp, "D:/ws", &sid);

    let usage = Usage {
        input: 100,
        output: 20,
        ..Default::default()
    };
    let provider = FakeProvider::new(vec![
        // 第二轮：收尾
        vec![
            ModelEvent::Start,
            ModelEvent::TextDelta(0, "工具结果已确认".into()),
            ModelEvent::Done(usage.clone(), StopReason::Stop),
        ],
        // 第一轮：正文 + 工具调用 + ToolUse
        vec![
            ModelEvent::Start,
            ModelEvent::TextDelta(0, "我先调用工具".into()),
            ModelEvent::ToolCallEnd(
                1,
                ToolCallDone {
                    call_id: "call_1".into(),
                    name: "echo".into(),
                    arguments: r#"{"text":"你好"}"#.into(),
                },
            ),
            ModelEvent::Done(usage.clone(), StopReason::ToolUse),
        ],
    ]);

    let cfg = agent_cfg(provider.clone());
    let events: Arc<Mutex<Vec<&'static str>>> = Arc::new(Mutex::new(Vec::new()));
    let sink = events.clone();
    let emit: wb_kernel::EmitFn = Arc::new(move |ev: &KernelEvent| {
        sink.lock().unwrap().push(event_name(ev));
    });
    let (_steer_tx, steer_rx) = tokio::sync::mpsc::channel(4);

    let outcome = wb_kernel::run(
        session,
        cfg,
        emit,
        CancellationToken::new(),
        steer_rx,
        "请调用 echo 工具".into(),
    )
    .await
    .expect("一轮运行不应失败");
    assert!(!outcome.aborted, "不该被中止");
    assert_eq!(outcome.last_text, "工具结果已确认", "最后一轮正文应回传");

    // 事件顺序契约：assistant_final 先于 message_end；工具批夹在两 turn 之间；settled 只在结算后
    let names = events.lock().unwrap().clone();
    let expect = [
        "agent_start",
        "turn_start",
        "text_delta",
        "assistant_final",
        "message_end",
        "tool_start",
        "tool_end",
        "turn_end",
        "turn_start",
        "text_delta",
        "assistant_final",
        "message_end",
        "turn_end",
        "agent_end",
        "settled",
    ];
    assert_eq!(names, expect, "事件顺序契约被破坏：界面时序依赖它");

    // 落盘：重放应含 user → assistant(工具调用) → tool(结果) → assistant(正文)
    let file = std::fs::read_dir(&tmp)
        .unwrap()
        .flatten()
        .map(|e| e.path())
        .find(|p| p.extension().map(|x| x == "jsonl").unwrap_or(false))
        .expect("应有会话文件");
    let loaded = wb_kernel::SessionFile::load(&file).expect("会话文件应可加载");
    let msgs = loaded.replay_messages();
    let roles: Vec<&'static str> = msgs
        .iter()
        .map(|m| match m {
            wb_kernel::Msg::User { .. } => "user",
            wb_kernel::Msg::Assistant { .. } => "assistant",
            wb_kernel::Msg::ToolResult { .. } => "tool",
        })
        .collect();
    assert_eq!(
        roles,
        ["user", "assistant", "tool", "assistant"],
        "回放序错乱"
    );
    if let Some(wb_kernel::Msg::ToolResult { content, .. }) = msgs.get(2) {
        assert_eq!(content, "你好", "工具结果应原样回灌");
    } else {
        panic!("第三条应是工具结果");
    }
    // 第二轮请求应把工具结果回灌（消息数：system+user+assistant+tool = 4）
    let second = &provider.seen.lock().unwrap()[1];
    assert_eq!(second.messages.len(), 4, "工具结果必须紧跟其调用回灌");

    let _ = std::fs::remove_dir_all(&tmp);
}

/// 并行工具调用：一轮里两个调用都要执行、都要落盘、都要回灌给下一轮。
/// 漏掉一个就是丢掉一次调用——模型以为做过了，实际没做。
#[tokio::test]
async fn parallel_tool_calls_are_all_executed_and_replayed() {
    let tmp = temp_dir("parallel");
    let provider = FakeProvider::new(vec![
        // 第二轮：收尾
        vec![
            ModelEvent::Start,
            ModelEvent::TextDelta(0, "都查完了".into()),
            ModelEvent::Done(Usage::default(), StopReason::Stop),
        ],
        // 第一轮：两个并行调用
        vec![
            ModelEvent::Start,
            ModelEvent::ToolCallEnd(
                0,
                ToolCallDone {
                    call_id: "c0".into(),
                    name: "echo".into(),
                    arguments: r#"{"text":"A"}"#.into(),
                },
            ),
            ModelEvent::ToolCallEnd(
                1,
                ToolCallDone {
                    call_id: "c1".into(),
                    name: "echo".into(),
                    arguments: r#"{"text":"B"}"#.into(),
                },
            ),
            ModelEvent::Done(Usage::default(), StopReason::ToolUse),
        ],
    ]);

    let cfg = agent_cfg(provider);
    let started: Arc<Mutex<Vec<String>>> = Arc::new(Mutex::new(Vec::new()));
    let sink = started.clone();
    let emit: wb_kernel::EmitFn = Arc::new(move |ev: &KernelEvent| {
        if let KernelEvent::ToolExecutionStart { args, .. } = ev {
            sink.lock()
                .unwrap()
                .push(args["text"].as_str().unwrap_or("").to_string());
        }
    });
    let (_tx, rx) = tokio::sync::mpsc::channel(4);
    let sess = wb_kernel::SessionFile::create(&tmp, "D:/ws", "SESS_par");
    wb_kernel::run(
        sess,
        cfg,
        emit,
        CancellationToken::new(),
        rx,
        "并行查两个".into(),
    )
    .await
    .expect("不应失败");

    let mut seen = started.lock().unwrap().clone();
    seen.sort();
    assert_eq!(
        seen,
        vec!["A".to_string(), "B".to_string()],
        "两个调用都要执行"
    );

    // 落盘：两个工具结果都要在回放里。
    let file = std::fs::read_dir(&tmp)
        .unwrap()
        .flatten()
        .map(|e| e.path())
        .find(|p| p.extension().map(|x| x == "jsonl").unwrap_or(false))
        .expect("应有会话文件");
    let loaded = wb_kernel::SessionFile::load(&file).unwrap();
    let replayed = loaded.replay_messages();
    let results: Vec<&str> = replayed
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::ToolResult { content, .. } => Some(content.as_str()),
            _ => None,
        })
        .collect();
    assert_eq!(results.len(), 2, "两个工具结果都要落盘，实际 {:?}", results);

    let _ = std::fs::remove_dir_all(&tmp);
}

/// 会话树：追加 → 重载 → 回放必须一字不差；压缩记录以摘要形态替代被覆盖的早期条目。
#[test]
fn session_tree_survives_reload_and_compaction_replaces_history() {
    let dir = temp_dir("session");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_TEST");
    sf.append(wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: "hi".into(),
            timestamp: 1,
        },
    })
    .unwrap();
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
    assert_eq!(sf.replay_messages().len(), 2);

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

    let _ = std::fs::remove_dir_all(&dir);
}

/// 上下文编辑（ContextEdit）把目标条目从回放里摘掉——压缩与回溯都靠它。
#[test]
fn context_edit_drops_targeted_entry() {
    let dir = temp_dir("edit");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_TEST2");
    let target = sf
        .append(wb_kernel::Entry::Message {
            message: wb_kernel::Msg::User {
                content: "bad".into(),
                timestamp: 1,
            },
        })
        .unwrap()
        .entry_id
        .clone();
    sf.append(wb_kernel::Entry::ContextEdit {
        target_entry_id: target,
        replacement: None,
    })
    .unwrap();
    assert!(
        sf.replay_messages().is_empty(),
        "被编辑的条目不应出现在回放里"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

/// 子代理跑完只把结论带回来，中间过程留在子会话文件里。
#[tokio::test]
async fn subagent_isolates_context_and_returns_conclusion() {
    let tmp = temp_dir("subagent");
    let usage = Usage {
        input: 30,
        output: 8,
        ..Default::default()
    };
    let provider = FakeProvider::new(vec![
        // 第二轮：收尾结论
        vec![
            ModelEvent::Start,
            ModelEvent::TextDelta(0, "共找到 3 个匹配文件".into()),
            ModelEvent::Done(usage.clone(), StopReason::Stop),
        ],
        // 第一轮：调工具
        vec![
            ModelEvent::Start,
            ModelEvent::ToolCallEnd(
                1,
                ToolCallDone {
                    call_id: "c1".into(),
                    name: "echo".into(),
                    arguments: r#"{"text":"扫描结果"}"#.into(),
                },
            ),
            ModelEvent::Done(usage.clone(), StopReason::ToolUse),
        ],
    ]);

    let mut cfg = agent_cfg(provider.clone());
    cfg.system_prompt = "父助手提示词\n<cwd>D:/ws</cwd>\n<skills>\n- demo\n</skills>".into();

    let sc = SubagentConfig {
        parent_session_id: "SESS_parent".into(),
        role_prompt: "你是探查员".into(),
        task: "扫描工作区并汇总".into(),
        tool_names: vec!["echo".into()],
        sessions_dir: tmp.clone(),
        workspace: "D:/ws".into(),
        max_turns: 8,
    };

    let outcome = wb_kernel::run_subagent(&cfg, sc, CancellationToken::new())
        .await
        .expect("子代理不应失败");
    assert!(!outcome.failed, "不该标记失败");
    assert_eq!(outcome.conclusion, "共找到 3 个匹配文件", "结论应原样带回");
    assert!(
        outcome.session_id.starts_with("SUB_"),
        "会话 id 应带 SUB_ 前缀"
    );
    assert_eq!(outcome.usage.input, 60, "两轮用量应累加");

    // 隔离：子会话文件在独立目录里，父上下文看不到它的中间过程。
    let files: Vec<_> = std::fs::read_dir(&tmp)
        .unwrap()
        .flatten()
        .map(|e| e.path())
        .filter(|p| p.extension().map(|x| x == "jsonl").unwrap_or(false))
        .collect();
    assert_eq!(files.len(), 1, "应只落一个子会话文件");
    let loaded = wb_kernel::SessionFile::load(&files[0]).expect("子会话文件应可加载");
    let msgs = loaded.replay_messages();
    assert_eq!(msgs.len(), 4, "子会话应含 user→assistant→tool→assistant");
    if let Some(wb_kernel::Msg::ToolResult { content, .. }) = msgs.get(2) {
        assert_eq!(content, "扫描结果", "工具结果应留在子会话里");
    } else {
        panic!("第三条应是工具结果");
    }

    // 子代理提示词带角色与父提示词的工作区段，但不带父会话的完整提示词正文。
    let first = &provider.seen.lock().unwrap()[0];
    let sys = match &first.messages[0] {
        wb_provider::ReqMessage {
            role: wb_provider::Role::System,
            parts,
            ..
        } => parts
            .iter()
            .map(|p| match p {
                wb_provider::ContentPart::Text(t) => t.clone(),
                _ => String::new(),
            })
            .collect::<String>(),
        _ => panic!("首条应是系统消息"),
    };
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
    let provider = FakeProvider::new(vec![vec![
        ModelEvent::Start,
        ModelEvent::TextDelta(0, "结论".into()),
        ModelEvent::Done(Usage::default(), StopReason::Stop),
    ]]);
    let mut cfg = agent_cfg(provider.clone());
    cfg.system_prompt = "父提示词".into();
    cfg.enabled_tools = vec!["echo".into(), "task".into()];

    let sc = SubagentConfig {
        parent_session_id: "SESS_parent".into(),
        role_prompt: String::new(),
        task: "干活".into(),
        tool_names: vec!["echo".into(), "task".into()],
        sessions_dir: tmp.clone(),
        workspace: "D:/ws".into(),
        max_turns: 4,
    };
    let _ = wb_kernel::run_subagent(&cfg, sc, CancellationToken::new())
        .await
        .expect("子代理不应失败");

    let first = &provider.seen.lock().unwrap()[0];
    let names: Vec<&str> = first.tools.iter().map(|t| t.name.as_str()).collect();
    assert_eq!(names, vec!["echo"], "task 应被剔除，避免子代理再派子代理");

    let _ = std::fs::remove_dir_all(&tmp);
}

/// 轮次用尽时回退到子代理最后说的正文，不给父会话一句空话。
#[tokio::test]
async fn subagent_falls_back_to_last_text_when_turn_limit_hit() {
    let tmp = temp_dir("subagent-limit");
    // 每轮都是「继续调工具」，永远不收尾——轮次上限是唯一的出口。
    let provider = FakeProvider::new(vec![
        vec![
            ModelEvent::Start,
            ModelEvent::TextDelta(0, "我先看看文件".into()),
            ModelEvent::ToolCallEnd(
                1,
                ToolCallDone {
                    call_id: "c1".into(),
                    name: "echo".into(),
                    arguments: r#"{"text":"x"}"#.into(),
                },
            ),
            ModelEvent::Done(Usage::default(), StopReason::ToolUse),
        ],
        vec![
            ModelEvent::Start,
            ModelEvent::ToolCallEnd(
                2,
                ToolCallDone {
                    call_id: "c2".into(),
                    name: "echo".into(),
                    arguments: r#"{"text":"y"}"#.into(),
                },
            ),
            ModelEvent::Done(Usage::default(), StopReason::ToolUse),
        ],
    ]);

    let cfg = agent_cfg(provider);
    let sc = SubagentConfig {
        parent_session_id: "SESS_parent".into(),
        role_prompt: String::new(),
        task: "干活".into(),
        tool_names: vec!["echo".into()],
        sessions_dir: tmp.clone(),
        workspace: "D:/ws".into(),
        max_turns: 2,
    };
    let outcome = wb_kernel::run_subagent(&cfg, sc, CancellationToken::new())
        .await
        .expect("子代理不应报错");
    assert!(
        outcome.conclusion.contains("我先看看文件"),
        "轮次用尽应回退到最后的正文，实际：{}",
        outcome.conclusion
    );

    let _ = std::fs::remove_dir_all(&tmp);
}
