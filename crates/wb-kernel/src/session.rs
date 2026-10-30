//! 会话 JSONL：只追加文件 + 树形条目 + leaf→root 回放（specs/05）。

use std::io::Write;
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};

use wb_pkg::{AppError, Result};
use wb_provider::Usage;

/// 会话文件首行。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SessionHeader {
    /// 类型标记（恒为 "session"）
    #[serde(rename = "type")]
    pub type_: String,
    /// 格式版本
    pub version: i64,
    /// 会话 id（SESS_ 前缀）
    pub session_id: String,
    /// 起建毫秒
    pub created_at: i64,
    /// 工作目录
    pub workspace: String,
}

/// 消息内容块。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum Block {
    /// 正文
    Text {
        /// 文本
        text: String,
    },
    /// 思考
    Thinking {
        /// 思考内容
        thinking: String,
    },
    /// 工具调用
    ToolCall {
        /// 调用 id（TC_ 前缀）
        tool_call_id: String,
        /// 工具名
        name: String,
        /// 参数 JSON 文本
        arguments: String,
    },
}

/// 消息体（role 区分四类）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "role", rename_all = "snake_case")]
pub enum Msg {
    /// 用户消息
    User {
        /// 文本
        content: String,
        /// 毫秒
        timestamp: i64,
    },
    /// 助手消息
    Assistant {
        /// 内容块（有序）
        content: Vec<Block>,
        /// provider id
        provider_id: String,
        /// 模型 id
        model_id: String,
        /// 消息 id（MSG_ 前缀）
        message_id: String,
        /// 用量
        #[serde(default)]
        usage: Option<Usage>,
        /// 停止原因：stop / length / tool_use / error / aborted
        stop_reason: String,
        /// 毫秒
        timestamp: i64,
        /// 耗时毫秒
        #[serde(default)]
        duration_ms: i64,
    },
    /// 工具结果
    ToolResult {
        /// 对应调用 id
        tool_call_id: String,
        /// 工具名
        tool_name: String,
        /// 结果文本（已截断）
        content: String,
        /// 是否错误
        is_error: bool,
        /// 毫秒
        timestamp: i64,
        /// 耗时
        #[serde(default)]
        duration_ms: i64,
    },
}

/// 条目载荷（type 已在外层 tag）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Entry {
    /// 消息
    #[serde(rename = "message")]
    Message {
        /// 消息体
        message: Msg,
    },
    /// 模型切换
    #[serde(rename = "model_change")]
    ModelChange {
        /// provider 键
        provider_id: String,
        /// 模型 id
        model_id: String,
    },
    /// 思考档位变更
    #[serde(rename = "thinking_change")]
    ThinkingChange {
        /// 档位
        thinking_level: String,
    },
    /// 压缩记录
    #[serde(rename = "compaction")]
    Compaction {
        /// 结构化摘要
        summary: String,
        /// 保留起点条目 id
        first_kept_entry_id: String,
        /// 压缩前 token
        tokens_before: i64,
    },
    /// 不进上下文的用量（重试剔除等）
    #[serde(rename = "usage")]
    UsageEntry {
        /// 记录种类
        kind: String,
        /// 用量
        usage: Usage,
    },
    /// 功能状态持久化（不进上下文）
    #[serde(rename = "custom")]
    Custom {
        /// 功能命名空间（如 todo）
        custom_type: String,
        /// 载荷
        data: serde_json::Value,
    },
    /// 追加式修改早前条目（replacement 为 null = 剔出上下文）
    #[serde(rename = "context_edit")]
    ContextEdit {
        /// 目标条目 id
        target_entry_id: String,
        /// 新内容（null = 剔除）
        replacement: Option<serde_json::Value>,
    },
    /// 会话显示名
    #[serde(rename = "session_info")]
    SessionInfo {
        /// 名称
        session_name: Option<String>,
    },
}

/// 条目外壳：公共基座 + 载荷。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EntryWrap {
    /// 类型标记
    #[serde(rename = "type")]
    pub type_: String,
    /// 条目 id（ENT_ 前缀）
    pub entry_id: String,
    /// 父条目 id（首条为 null）
    pub parent_entry_id: Option<String>,
    /// 毫秒
    pub timestamp: i64,
    /// 载荷（扁平合入同一行）
    #[serde(flatten)]
    pub payload: Entry,
}

impl EntryWrap {
    /// 构造（自动生成 id 与时间）。
    pub fn new(parent: Option<&str>, payload: Entry) -> Self {
        Self {
            type_: payload_type(&payload).to_string(),
            entry_id: wb_pkg::ulid::new_id(wb_pkg::Prefix::Ent),
            parent_entry_id: parent.map(|s| s.to_string()),
            timestamp: now_ms(),
            payload,
        }
    }
}

/// 载荷 → 类型标记。
fn payload_type(p: &Entry) -> &'static str {
    match p {
        Entry::Message { .. } => "message",
        Entry::ModelChange { .. } => "model_change",
        Entry::ThinkingChange { .. } => "thinking_change",
        Entry::Compaction { .. } => "compaction",
        Entry::UsageEntry { .. } => "usage",
        Entry::Custom { .. } => "custom",
        Entry::ContextEdit { .. } => "context_edit",
        Entry::SessionInfo { .. } => "session_info",
    }
}

/// 当前毫秒。
pub fn now_ms() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// 一个会话文件的句柄：加载（容忍坏行）、追加、回放。
pub struct SessionFile {
    /// 文件绝对路径
    pub path: PathBuf,
    /// 首行
    pub header: SessionHeader,
    /// 全部条目（顺序即写入序）
    pub entries: Vec<EntryWrap>,
    /// 当前叶子（树指针）
    pub leaf: Option<String>,
    /// 写句柄（懒开：首条消息时建文件）
    writer: Option<std::fs::File>,
}

impl SessionFile {
    /// 新建会话（不立即落盘：首条消息到达时才建文件，避免空会话垃圾）。
    pub fn create(sessions_dir: &Path, workspace: &str, session_id: &str) -> Self {
        Self {
            path: sessions_dir.join(format!("{}_{}.jsonl", now_ms(), session_id)),
            header: SessionHeader {
                type_: "session".into(),
                version: 1,
                session_id: session_id.to_string(),
                created_at: now_ms(),
                workspace: workspace.to_string(),
            },
            entries: Vec::new(),
            leaf: None,
            writer: None,
        }
    }

    /// 加载已有会话（坏行跳过并计数；用于界面提示 9701）。
    pub fn load(path: &Path) -> Result<Self> {
        let text = std::fs::read_to_string(path)
            .map_err(|e| AppError::wrap("1002", "读会话文件失败", e))?;
        let mut lines = text.lines();
        let header_line = lines.next().unwrap_or("");
        let header: SessionHeader = serde_json::from_str(header_line)
            .map_err(|e| AppError::wrap(wb_domain::error::kernel::FILE_CORRUPT, "会话头损坏", e))?;
        let mut entries = Vec::new();
        let mut bad = 0usize;
        for line in lines {
            if line.trim().is_empty() {
                continue;
            }
            match serde_json::from_str::<EntryWrap>(line) {
                Ok(e) => entries.push(e),
                Err(_) => bad += 1,
            }
        }
        if bad > 0 {
            tracing::warn!(file = %path.display(), bad, "会话文件存在坏行，已跳过");
        }
        let leaf = entries.last().map(|e| e.entry_id.clone());
        Ok(Self {
            path: path.to_path_buf(),
            header,
            entries,
            leaf,
            writer: None,
        })
    }

    /// 追加条目（首条时建文件写 header；同窗口期同步 flush）。
    pub fn append(&mut self, payload: Entry) -> Result<&EntryWrap> {
        if self.writer.is_none() {
            if let Some(parent) = self.path.parent() {
                std::fs::create_dir_all(parent)
                    .map_err(|e| AppError::wrap("2004", "数据目录不可写", e))?;
            }
            let mut f = std::fs::OpenOptions::new()
                .create(true)
                .append(true)
                .open(&self.path)
                .map_err(|e| AppError::wrap("1002", "建会话文件失败", e))?;
            let head = serde_json::to_string(&self.header)
                .map_err(|e| AppError::wrap("2001", "序列化失败", e))?;
            writeln!(f, "{head}").map_err(|e| AppError::wrap("1002", "写会话失败", e))?;
            self.writer = Some(f);
        }
        let entry = EntryWrap::new(self.leaf.as_deref(), payload);
        let line =
            serde_json::to_string(&entry).map_err(|e| AppError::wrap("2001", "序列化失败", e))?;
        let f = self.writer.as_mut().unwrap();
        writeln!(f, "{line}").map_err(|e| AppError::wrap("1002", "写会话失败", e))?;
        f.flush().ok();
        self.leaf = Some(entry.entry_id.clone());
        self.entries.push(entry);
        Ok(self.entries.last().unwrap())
    }

    /// leaf→root 路径上的条目（顺序从根到叶）。
    pub fn path_to_leaf(&self) -> Vec<&EntryWrap> {
        let by_id: std::collections::HashMap<&str, &EntryWrap> = self
            .entries
            .iter()
            .map(|e| (e.entry_id.as_str(), e))
            .collect();
        let mut chain = Vec::new();
        let mut cur = self.leaf.as_deref();
        while let Some(id) = cur {
            match by_id.get(id) {
                Some(e) => {
                    chain.push(*e);
                    cur = e.parent_entry_id.as_deref();
                }
                None => break,
            }
        }
        chain.reverse();
        chain
    }

    /// 回放为消息序列：套 context_edit 剔除，compaction 摘要以 user 消息形态插回。
    pub fn replay_messages(&self) -> Vec<Msg> {
        let mut dropped: std::collections::HashSet<String> = std::collections::HashSet::new();
        for e in self.path_to_leaf() {
            if let Entry::ContextEdit {
                target_entry_id,
                replacement: None,
            } = &e.payload
            {
                dropped.insert(target_entry_id.clone());
            }
        }
        let path = self.path_to_leaf();
        // 最新 compaction 及其保留点
        let compact_at = path
            .iter()
            .rposition(|e| matches!(e.payload, Entry::Compaction { .. }));
        let mut out = Vec::new();
        for (i, e) in path.iter().enumerate() {
            if let Some(ci) = compact_at {
                if i < ci {
                    continue; // 压缩点之前的原始消息全部省略（摘要已覆盖）
                }
                if i == ci {
                    if let Entry::Compaction { summary, .. } = &e.payload {
                        out.push(Msg::User {
                            content: format!(
                                "The conversation history before this point was compacted into the following summary:\n\n<summary>\n{summary}\n</summary>"
                            ),
                            timestamp: e.timestamp,
                        });
                    }
                    continue;
                }
            }
            if dropped.contains(&e.entry_id) {
                continue;
            }
            if let Entry::Message { message } = &e.payload {
                out.push(message.clone());
            }
        }
        out
    }
}
