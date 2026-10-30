//! 资源类类型：文件树、帮助文档、斜杠命令、工作区历史、背景图、上下文分解。

use serde::{Deserialize, Serialize};

/// 文件树条目。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct FileEntryVO {
    /// 文件或目录名
    pub entry_name: String,
    /// 相对工作区路径
    pub entry_path: String,
    /// 是否目录
    pub entry_is_dir: bool,
    /// 字节数（目录为 0）
    pub entry_size: i64,
    /// 修改时间（毫秒）
    pub entry_mtime: i64,
}

/// 文件清单响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct FileListRESP {
    /// 目录本体的相对路径
    pub resp_path: String,
    /// 子项（目录在前，名字序）
    pub resp_entries: Vec<FileEntryVO>,
}

/// 文件字节响应（文档预览：PDF / Word / Excel 都靠它）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct FileBytesRESP {
    /// 文件名（带扩展名，前端据此选渲染器）
    pub resp_file_name: String,
    /// MIME 类型
    pub resp_mime: String,
    /// 文件字节的 base64（JSON 传输二进制的安全形式）
    pub resp_content_base64: String,
    /// 字节数
    pub resp_size: i64,
}

/// 帮助文档条目。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct HelpDocVO {
    /// 文档名（URL 段，不含扩展名）
    pub doc_name: String,
    /// 标题（取首个一级标题，缺省用文件名）
    pub doc_title: String,
}

/// 帮助文档正文响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct HelpDocRESP {
    /// 文档名
    pub resp_name: String,
    /// markdown 原文
    pub resp_content: String,
}

/// 斜杠命令条目。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct CommandVO {
    /// 命令名（不含斜杠）
    pub command_name: String,
    /// 说明
    pub command_description: String,
    /// skill / feature
    pub command_source: String,
}

/// 工作区历史条目。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct WorkspaceVO {
    /// 工作区绝对路径
    pub workspace_path: String,
    /// 最近使用时间（毫秒）
    pub workspace_last_used: i64,
}

/// 工作区历史响应。
#[derive(Debug, Clone, Default, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct WorkspacesRESP {
    /// 按最近使用排序
    pub resp_workspaces: Vec<WorkspaceVO>,
}

/// 背景图设置响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct BackgroundVO {
    /// 图片名（空 = 未设置）；本体走 /assets/background
    pub resp_background_file: String,
    /// 不透明度 0-100
    pub resp_background_opacity: i64,
    /// 模糊像素
    pub resp_background_blur: i64,
}

/// 背景图设置请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct BackgroundSaveREQ {
    /// 图片绝对路径；空串 = 清除
    pub req_background_path: String,
    /// 不透明度 0-100
    pub req_background_opacity: i64,
    /// 模糊像素
    pub req_background_blur: i64,
}

/// 上下文分解段。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ContextSegmentVO {
    /// 段名（系统提示词 / 工具定义 / 历史消息…）
    pub segment_name: String,
    /// 估算 token
    pub segment_tokens: i64,
}

/// 上下文占用分解响应（context_view 数据源）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ContextBreakdownRESP {
    /// 分段明细
    pub resp_segments: Vec<ContextSegmentVO>,
    /// 估算总占用
    pub resp_total_tokens: i64,
    /// 上下文窗口
    pub resp_context_window: i64,
    /// 占比 0-1
    pub resp_ratio: f64,
}

/// 自省快照（inspect 功能的数据面）：当前装配一屏看全。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct InspectSnapshotRESP {
    /// 全部工具（含停用）
    pub resp_tools: Vec<serde_json::Value>,
    /// 全部技能（含停用）
    pub resp_skills: Vec<serde_json::Value>,
    /// 斜杠命令
    pub resp_commands: Vec<serde_json::Value>,
    /// 当前组装的系统提示词全文
    pub resp_system_prompt: String,
    /// 功能开关快照
    pub resp_features: Vec<serde_json::Value>,
    /// 会话元信息（带 session_id 时）
    pub resp_session: Option<serde_json::Value>,
}
