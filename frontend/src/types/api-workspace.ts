// 前端与后端的契约类型（工作区：文件 / git）。字段名与 Go 侧 json tag 一一对应。

import type { DiffLine } from './api-chat'

// ---------------- 文件 ----------------

export interface FileEntry {
  entry_name: string
  entry_is_dir: boolean
  size_bytes: number
  modified_at: number
}

export interface FileList {
  dir_path: string
  entries: FileEntry[]
  truncated: boolean
}

/** 全工作区搜索的一个命中：@ 引用直接用 file_path */
export interface FileHit {
  file_path: string
  entry_name: string
}

export interface FileSearch {
  search_query: string
  hits: FileHit[]
  truncated: boolean
}

export interface FileRead {
  file_path: string
  file_text: string
  size_bytes: number
  is_binary: boolean
  truncated: boolean
}

// ---------------- Git ----------------

export type GitChange = 'M' | 'A' | 'D' | 'R' | 'C' | 'T' | '?' | 'U'

export interface GitFile {
  file_path: string
  change_code: GitChange
  is_staged: boolean
  has_conflict: boolean
  insertions: number
  deletions: number
}

export interface GitRemote {
  remote_name: string
  remote_url: string
}

export interface GitStatus {
  git_available: boolean
  unavailable_reason: string
  is_repo: boolean
  workspace: string
  repo_root: string
  branch_name: string
  upstream_ref: string
  ahead: number
  behind: number
  repo_state: string
  repo_state_detail: string
  remotes: GitRemote[]
  changed_files: GitFile[]
  has_conflict: boolean
  git_version: string
}

export interface GitDiff {
  file_path: string
  truncated: boolean
  dropped: number
  diff_lines: DiffLine[]
}

export interface GitCommit {
  commit_hash: string
  short_hash: string
  commit_subject: string
  author_name: string
  commit_time: number
  file_count: number
  insertions: number
  deletions: number
}

/** 单个提交的完整内容：元信息 + 改动文件 + 逐行差异。 */
export interface GitCommitDetail {
  commit: GitCommit
  commit_body: string
  parent_hashes: string[]
  changed_files: GitFile[]
  diff: GitDiff
}

export interface GitBranch {
  branch_name: string
  is_current: boolean
  remote_name?: string
  ahead: number
  behind: number
}

export interface GitCommitResult {
  status: GitStatus
  commit_hash: string
}

export interface GitStash {
  stash_ref: string
  stash_subject: string
}
