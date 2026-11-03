// Git 工作区视图的类型：status / diff / commit / branch / push / pull。
// 从 api.ts 拆出：它随 git 面板一起演进；api.ts 末尾重新导出，调用方无感。

// ---- Git（工作区版本视图）----

export interface GitFileVO {
  git_path: string
  /** M 改过 / A 新增 / D 删除 / R 改名 / C 复制 / T 类型变化 / ? 未跟踪 / U 冲突 */
  git_status: string
  git_staged: boolean
  /** 该文件有未解决冲突；解决后要显式标记，否则 git 不接受提交 */
  git_conflict: boolean
  git_additions: number
  git_deletions: number
}

export interface GitRemoteVO {
  git_remote_name: string
  git_remote_url: string
}

export interface GitStatusVO {
  git_available: boolean
  git_reason: string
  git_is_repo: boolean
  git_workspace: string
  /** 仓库根；工作区是仓库子目录时与 workspace 不同 */
  git_repo_root: string
  git_branch: string
  /** 上游引用（如 origin/main）；没有上游为空 */
  git_upstream: string
  git_ahead: number
  git_behind: number
  /** none / merge / rebase / cherry-pick / revert */
  git_state: string
  git_state_detail: string
  git_remotes: GitRemoteVO[]
  git_files: GitFileVO[]
  git_conflict: boolean
  git_version: string
}

export interface GitDiffLineVO {
  git_line_kind: string
  git_line_old_no: number
  git_line_new_no: number
  git_line_text: string
}

export interface GitDiffVO {
  git_diff_path: string
  git_truncated: boolean
  git_dropped: number
  git_lines: GitDiffLineVO[]
}

export interface GitCommitVO {
  git_hash: string
  git_short: string
  git_subject: string
  git_author: string
  /** 提交时间（毫秒） */
  git_when: number
  git_files: number
  git_additions: number
  git_deletions: number
}

export interface GitCommitDetailVO {
  git_hash: string
  git_short: string
  git_subject: string
  git_body: string
  git_author: string
  git_when: number
  git_parents: string[]
  git_files: GitFileVO[]
  /** 单文件详情时附带的 diff */
  git_diff?: GitDiffVO
}

export interface GitBranchVO {
  git_name: string
  git_current: boolean
  /** 远端分支所属远端名；本地分支为空 */
  git_remote: string
  git_ahead: number
  git_behind: number
}

/** 推送的强制级别。默认 none，逐级需要更多确认 */
export type GitForce = 'none' | 'lease' | 'force'

export interface GitPushREQ {
  git_remote?: string
  git_branch?: string
  /** 把这次推送的远端建立为上游（首次推送必须） */
  git_set_upstream?: boolean
  git_force?: GitForce
  /** 强制推送时必须与分支名完全一致的分支名 */
  git_confirm?: string
  git_session_id?: string
}

export interface GitPullREQ {
  git_remote?: string
  git_branch?: string
  git_rebase?: boolean
  git_session_id?: string
}

export interface GitCommitRESP {
  resp_status: GitStatusVO
  resp_hash: string
}

export interface GitMessageRESP {
  resp_message: string
  resp_err: string
}


// 功能面板相关的类型拆到 featurePanel.ts，这里统一再导出，调用方无感。
