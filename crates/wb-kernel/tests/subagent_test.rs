//! 子代理测试：隔离子会话 + 工具白名单收窄 + 结论回灌。

use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use futures::stream;
use tokio_util::sync::CancellationToken;
use wb_kernel::{AgentConfig, SubagentConfig};
use wb_provider::{
    ModelClient, ModelEvent, ProviderStream, StopReason, StreamRequest, ThinkingLevel, ToolCallDone,
    Usage,
};
use wb_tools::{Tool, ToolOutput, ToolRegistry};

/// 脚本化假 Provider：每次 stream() 弹出一段预设事件流。
struct FakeProvider {
    /// 每轮的事件脚本（依次弹出）
    turns: Mutex<Vec<Vec<ModelEvent>>>,
    /// 收到的请求（供断言工具声明）
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

/// 子代理跑完只把结论带回来，中间过程留在子会话文件里。
#[tokio::test]
async fn subagent_isolates_context_and_returns_conclusion() {
    let tmp = std::env::temp_dir().join(format!("wb-subagent-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&tmp);
    std::fs::create_dir_all(&tmp).unwrap();

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

    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(EchoTool));
    let cfg = AgentConfig {
        client: provider.clone(),
        registry: Arc::new(registry),
        enabled_tools: vec!["echo".into()],
        system_prompt: "父助手提示词\n<cwd>D:/ws</cwd>\n<skills>\n- demo\n</skills>".into(),
        provider_id: "test".into(),
        model_id: "fake-model".into(),
        context_window: 100_000,
        max_tokens: 1024,
        thinking: ThinkingLevel::Off,
        max_retries: 0,
    };

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
    assert!(outcome.session_id.starts_with("SUB_"), "会话 id 应带 SUB_ 前缀");
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
    assert!(!sys.contains("父助手提示词"), "不应把父提示词正文整段灌进子代理");

    let _ = std::fs::remove_dir_all(&tmp);
}

/// 工具白名单收窄：子代理拿不到父会话之外的工具，也不能再派子代理。
#[tokio::test]
async fn subagent_tool_list_is_narrowed() {
    let tmp = std::env::temp_dir().join(format!("wb-subagent-tools-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&tmp);
    std::fs::create_dir_all(&tmp).unwrap();

    let provider = FakeProvider::new(vec![vec![
        ModelEvent::Start,
        ModelEvent::TextDelta(0, "结论".into()),
        ModelEvent::Done(Usage::default(), StopReason::Stop),
    ]]);
    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(EchoTool));
    let cfg = AgentConfig {
        client: provider.clone(),
        registry: Arc::new(registry),
        enabled_tools: vec!["echo".into(), "task".into()],
        system_prompt: "父提示词".into(),
        provider_id: "test".into(),
        model_id: "fake-model".into(),
        context_window: 100_000,
        max_tokens: 1024,
        thinking: ThinkingLevel::Off,
        max_retries: 0,
    };

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
    let tmp = std::env::temp_dir().join(format!("wb-subagent-limit-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&tmp);
    std::fs::create_dir_all(&tmp).unwrap();

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

    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(EchoTool));
    let cfg = AgentConfig {
        client: provider.clone(),
        registry: Arc::new(registry),
        enabled_tools: vec!["echo".into()],
        system_prompt: "父提示词".into(),
        provider_id: "test".into(),
        model_id: "fake-model".into(),
        context_window: 100_000,
        max_tokens: 1024,
        thinking: ThinkingLevel::Off,
        max_retries: 0,
    };

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