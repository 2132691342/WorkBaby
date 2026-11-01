//! Agent 内核：会话树（JSONL）、主循环、压缩、事件。内核只认工具与 Provider，不感知上层业务。

pub mod agent;
pub mod blocks;
pub mod compaction;
pub mod events;
pub mod session;
pub mod subagent;

pub use agent::{run, AgentConfig, RunOutcome};
pub use events::{EmitFn, KernelEvent};
pub use session::{now_ms, Block, Entry, EntryWrap, Msg, SessionFile};
pub use subagent::{run_subagent, ObserveFn, SubagentConfig, SubagentEvent, SubagentOutcome};
