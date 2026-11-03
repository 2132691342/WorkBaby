//! Agent 内核：会话树（JSONL）、主循环、压缩、事件。内核只认工具与 Provider，不感知上层业务。

pub mod agent;
pub mod blocks;
pub mod branch_summary;
pub mod compaction;
pub mod events;
pub mod reqbuild;
pub mod session;
pub mod session_msg;
pub mod session_repair;
pub mod session_tree;
pub mod subagent;
mod subagent_prompt;
pub mod throttle;
pub mod tokenest;
pub mod toolbatch;

/// 工具过程回调的类型别名（与 wb-tools 同一份定义，避免内核引两层）。
pub mod tools_progress {
    pub use wb_tools::ProgressFn;
}

pub use agent::{run, AgentConfig, RunOutcome};
pub use events::{EmitFn, KernelEvent};
pub use reqbuild::SkillLoader;
pub use session::{now_ms, Block, Entry, EntryWrap, ImagePart, Msg, SessionFile, UserInput};
pub use subagent::{run_subagent, ObserveFn, SubagentConfig, SubagentEvent, SubagentOutcome};
