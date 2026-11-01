//! 内核场景测试的共享夹具：假模型、假工具、临时目录与会话装配。
//!
//! 内核测试要控制「模型说什么、工具返回什么」，所以 provider 与工具都是假的；
//! 夹具留在这里，测试文件按被测行为分文件（见 AGENTS.md §3）。

#![allow(dead_code)]

use std::path::Path;
use std::sync::{Arc, Mutex};

pub fn delta(t: &str) -> ModelEvent {
    ModelEvent::TextDelta(0, t.into())
}

pub fn tool_end(idx: usize, id: &str, args: &str) -> ModelEvent {
    ModelEvent::ToolCallEnd(
        idx,
        ToolCallDone {
            call_id: id.into(),
            name: "echo".into(),
            arguments: args.into(),
        },
    )
}

pub fn done(input: i64, stop: StopReason) -> ModelEvent {
    ModelEvent::Done(
        Usage {
            input,
            ..Default::default()
        },
        stop,
    )
}

pub fn tool_use() -> ModelEvent {
    done(0, StopReason::ToolUse)
}

/// 脚本化假 Provider：stream() 弹出一段预设事件流，并记录收到的请求。
pub struct FakeProvider {
    pub turns: Mutex<Vec<Vec<ModelEvent>>>,
    pub seen: Mutex<Vec<StreamRequest>>,
}

impl FakeProvider {
    pub fn new(turns: Vec<Vec<ModelEvent>>) -> Arc<Self> {
        Arc::new(Self {
            turns: Mutex::new(turns),
            seen: Mutex::new(Vec::new()),
        })
    }

    pub fn first_system_text(&self) -> String {
        let seen = self.seen.lock().unwrap();
        let StreamRequest { messages, .. } = &seen[0];
        let ReqMessage {
            role: Role::System,
            parts,
            ..
        } = &messages[0]
        else {
            panic!("首条应是系统消息");
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
pub struct EchoTool;

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

pub fn event_name(ev: &KernelEvent) -> &'static str {
    match ev {
        KernelEvent::AgentStart { .. } => "agent_start",
        KernelEvent::TurnStart => "turn_start",
        KernelEvent::MessageEnd { .. } => "message_end",
        KernelEvent::TextDelta { .. } => "text_delta",
        KernelEvent::ThinkingDelta { .. } => "thinking_delta",
        KernelEvent::ToolExecutionStart { .. } => "tool_start",
        KernelEvent::ToolExecutionEnd { .. } => "tool_end",
        KernelEvent::TurnEnd { .. } => "turn_end",
        KernelEvent::CompactionStart { .. } => "compaction_start",
        KernelEvent::CompactionEnd { .. } => "compaction_end",
        KernelEvent::AutoRetry { .. } => "retry",
        KernelEvent::AssistantFinal { .. } => "assistant_final",
        KernelEvent::AgentEnd { .. } => "agent_end",
        KernelEvent::Settled { .. } => "settled",
        KernelEvent::Failed { .. } => "failed",
    }
}

pub fn temp_dir(tag: &str) -> std::path::PathBuf {
    let dir = std::env::temp_dir().join(format!("wb_kernel_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    dir
}

pub fn agent_cfg(provider: Arc<FakeProvider>) -> AgentConfig {
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
        reinforce: None,
        workdir: temp_dir("cfg"),
    }
}

pub fn load_session(dir: &Path) -> wb_kernel::SessionFile {
    let file = std::fs::read_dir(dir)
        .unwrap()
        .flatten()
        .map(|e| e.path())
        .find(|p| p.extension().is_some_and(|x| x == "jsonl"))
        .expect("应有会话文件");
    wb_kernel::SessionFile::load(&file).expect("会话文件应可加载")
}

pub fn role_names(msgs: &[wb_kernel::Msg]) -> Vec<&'static str> {
    msgs.iter()
        .map(|m| match m {
            wb_kernel::Msg::User { .. } => "user",
            wb_kernel::Msg::Assistant { .. } => "assistant",
            wb_kernel::Msg::ToolResult { .. } => "tool",
        })
        .collect()
}

pub fn sub_cfg(tmp: &Path, tools: &[&str]) -> SubagentConfig {
    SubagentConfig {
        parent_session_id: "SESS_parent".into(),
        role_code: "worker".into(),
        role_prompt: String::new(),
        task: "干活".into(),
        tool_names: tools.iter().map(|t| t.to_string()).collect(),
        sessions_dir: tmp.to_path_buf(),
        workspace: "D:/ws".into(),
        max_turns: 8,
        observe: None,
    }
}

// 子测试文件只写 use common::*，依赖全部由这里转发，避免每个文件抄一遍 use 清单。
pub use async_trait::async_trait;
pub use futures::stream;
pub use tokio_util::sync::CancellationToken;
pub use wb_kernel::{AgentConfig, KernelEvent, SubagentConfig};
pub use wb_provider::{
    ContentPart, ModelClient, ModelEvent, ProviderStream, ReqMessage, Role, StopReason,
    StreamRequest, ThinkingLevel, ToolCallDone, Usage,
};
pub use wb_tools::{Tool, ToolOutput, ToolRegistry};
