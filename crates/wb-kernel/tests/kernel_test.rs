//! 内核长链路测试：假 Provider 脚本回放，走「建会话→发送→工具→落盘→用量」全链路。

use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use futures::stream;
use tokio_util::sync::CancellationToken;
use wb_kernel::{AgentConfig, KernelEvent};
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

/// 全链路：两轮采样夹一次工具调用；顺序、落盘、用量三头验。
#[tokio::test]
async fn run_with_tool_call_replays_end_to_end() {
    let tmp = std::env::temp_dir().join(format!("wb-kernel-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&tmp);
    std::fs::create_dir_all(&tmp).unwrap();
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

    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(EchoTool));
    let cfg = AgentConfig {
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
    };

    let events: Arc<Mutex<Vec<&'static str>>> = Arc::new(Mutex::new(Vec::new()));
    let sink = events.clone();
    let emit: wb_kernel::EmitFn = Arc::new(move |ev: &KernelEvent| {
        sink.lock().unwrap().push(event_name(ev));
    });
    let (steer_tx, steer_rx) = tokio::sync::mpsc::channel(4);
    drop(steer_tx);

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
