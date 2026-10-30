//! 工作区版本视图类型：状态 / diff / 历史 / 分支 / 白名单写动作。时间一律毫秒整数。

use serde::{Deserialize, Serialize};

/// 状态清单里的一个文件条目（同一文件可因暂存与未暂存各出一条）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitFileVO {
    /// 相对工作区的路径
    pub git_path: String,
    /// M 改过 / A 新增 / D 删除 / R 改名 / C 复制 / T 类型变化 / ? 未跟踪 / U 冲突
    pub git_status: String,
    /// 变更是否已暂存
    pub git_staged: bool,
    /// 有未解决冲突；解决后要显式标记，否则 git 不接受提交
    pub git_conflict: bool,
    /// 新增行数
    pub git_additions: i64,
    /// 删除行数
    pub git_deletions: i64,
}

/// 远端条目。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitRemoteVO {
    /// 远端名
    pub git_remote_name: String,
    /// fetch 地址
    pub git_remote_url: String,
}

/// 工作区版本状态快照（/git/status 出参）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitStatusVO {
    /// git 可执行文件是否可用
    pub git_available: bool,
    /// 不可用原因（available=false 时展示）
    pub git_reason: String,
    /// 工作区是否是 git 仓库
    pub git_is_repo: bool,
    /// 工作区路径
    pub git_workspace: String,
    /// 仓库根；工作区是仓库子目录时与 workspace 不同
    pub git_repo_root: String,
    /// 当前分支；detached 为 "HEAD"
    pub git_branch: String,
    /// 上游引用（如 origin/main）；没有上游为空
    pub git_upstream: String,
    /// 领先上游提交数
    pub git_ahead: i64,
    /// 落后上游提交数
    pub git_behind: i64,
    /// none / merge / rebase / cherry-pick / revert
    pub git_state: String,
    /// 中间态补充说明（如「变基进行到第 2 步」）
    pub git_state_detail: String,
    /// 远端清单
    pub git_remotes: Vec<GitRemoteVO>,
    /// 变更文件清单
    pub git_files: Vec<GitFileVO>,
    /// 任一文件存在未解决冲突
    pub git_conflict: bool,
    /// git 版本串（available=false 时为空）
    pub git_version: String,
}

impl GitStatusVO {
    /// git 不可用或非仓库时的占位快照。
    pub fn unavailable(workspace: &str, reason: String, version: String) -> Self {
        Self {
            git_available: version.is_empty(),
            git_reason: reason,
            git_is_repo: false,
            git_workspace: workspace.to_string(),
            git_repo_root: String::new(),
            git_branch: String::new(),
            git_upstream: String::new(),
            git_ahead: 0,
            git_behind: 0,
            git_state: "none".into(),
            git_state_detail: String::new(),
            git_remotes: Vec::new(),
            git_files: Vec::new(),
            git_conflict: false,
            git_version: version,
        }
    }
}

/// diff 的一行。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitDiffLineVO {
    /// hunk / ctx / add / del / meta（\ No newline 等元行）
    pub git_line_kind: String,
    /// 旧文件行号（0 = 新增行）
    pub git_line_old_no: i64,
    /// 新文件行号（0 = 删除行）
    pub git_line_new_no: i64,
    /// 行文本（不含 +/- 前缀）
    pub git_line_text: String,
}

/// 单文件 diff（/git/diff 出参）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitDiffVO {
    /// 文件相对路径
    pub git_diff_path: String,
    /// 是否因行数上限被截断
    pub git_truncated: bool,
    /// 被截掉的行数
    pub git_dropped: i64,
    /// 行序列
    pub git_lines: Vec<GitDiffLineVO>,
}

/// 提交列表条目。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitCommitVO {
    /// 完整 hash
    pub git_hash: String,
    /// 短 hash
    pub git_short: String,
    /// 标题行
    pub git_subject: String,
    /// 作者名
    pub git_author: String,
    /// 提交时间（毫秒）
    pub git_when: i64,
    /// 变更文件数
    pub git_files: i64,
    /// 新增行数
    pub git_additions: i64,
    /// 删除行数
    pub git_deletions: i64,
}

/// 单提交详情。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitCommitDetailVO {
    /// 完整 hash
    pub git_hash: String,
    /// 短 hash
    pub git_short: String,
    /// 标题行
    pub git_subject: String,
    /// 正文（标题之外的部分）
    pub git_body: String,
    /// 作者名
    pub git_author: String,
    /// 提交时间（毫秒）
    pub git_when: i64,
    /// 父提交 hash
    pub git_parents: Vec<String>,
    /// 变更文件清单（含增删行数）
    pub git_files: Vec<GitFileVO>,
    /// 单文件详情时附带的 diff（给 path 参数才有）
    pub git_diff: Option<GitDiffVO>,
}

/// 分支条目。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitBranchVO {
    /// 分支名（远端分支带 origin/ 前缀）
    pub git_name: String,
    /// 是否当前分支
    pub git_current: bool,
    /// 所属远端名；本地分支为空
    pub git_remote: String,
    /// 领先上游提交数（仅本地分支）
    pub git_ahead: i64,
    /// 落后上游提交数（仅本地分支）
    pub git_behind: i64,
}

// ---- 请求 ----

/// 路径批量动作入参（stage / unstage / discard / resolve）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitPathsREQ {
    /// 相对工作区的文件路径
    pub git_paths: Vec<String>,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

/// 提交入参。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitCommitREQ {
    /// 提交信息（非空）
    pub git_message: String,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
    /// 仅 HEAD 未推送时允许 amend
    pub git_amend: bool,
}

/// 分支名入参（新建 / 切换 / 删除）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitNameREQ {
    /// 分支名
    pub git_name: String,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

/// 合并 / 变基入参。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitMergeREQ {
    /// 目标分支名
    pub git_name: String,
    /// true = 变基，false = 合并
    pub git_rebase: bool,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

/// 中间态继续 / 中止入参。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitStateREQ {
    /// continue / abort
    pub git_action: String,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

/// 拉取入参。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitPullREQ {
    /// 远端名（空 = 上游默认）
    pub git_remote: String,
    /// 远端分支名（空 = 上游默认）
    pub git_branch: String,
    /// true = 变基方式拉取
    pub git_rebase: bool,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

/// 推送强制级别。none 常规 / lease force-with-lease / force 裸 force（需 confirm）。
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitPushREQ {
    /// 远端名（空 = 上游默认）
    pub git_remote: String,
    /// 分支名（空 = 当前分支）
    pub git_branch: String,
    /// 首次推送建立上游
    pub git_set_upstream: bool,
    /// none / lease / force
    pub git_force: String,
    /// force 级别必须与分支名完全一致的确认串
    pub git_confirm: String,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

/// 仓库初始化入参。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitInitREQ {
    /// 初始分支名（空 = git 默认）
    pub git_branch: String,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

/// 远端登记入参。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GitRemoteREQ {
    /// 远端名（如 origin）
    pub git_name: String,
    /// 仓库地址
    pub git_url: String,
    /// 会话 id（当前一律用工作区）
    pub git_session_id: String,
}

// ---- 响应 ----

/// 提交结果：新快照 + 新提交 hash。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct GitCommitRESP {
    /// 提交后的状态快照
    pub resp_status: GitStatusVO,
    /// 新提交 hash
    pub resp_hash: String,
}

/// 提交信息起草结果。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct GitMessageRESP {
    /// 起草的信息；失败为空
    pub resp_message: String,
    /// 失败原因；成功为空
    pub resp_err: String,
}
