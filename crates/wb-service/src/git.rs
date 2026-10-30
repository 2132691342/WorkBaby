//! git 编排：工作区取自 Services，读写转发 wb-git；写动作完成后回吐新快照。

use std::path::PathBuf;

use wb_domain::git::{
    GitBranchVO, GitCommitDetailVO, GitCommitRESP, GitCommitVO, GitDiffVO, GitMessageRESP,
    GitPushREQ, GitStatusVO,
};
use wb_pkg::Result;

use super::Services;

/// 当前工作区目录。
fn ws_dir(sv: &Services) -> PathBuf {
    PathBuf::from(sv.workspace.lock().unwrap().clone())
}

/// 状态快照。
pub fn status(sv: &Services) -> GitStatusVO {
    wb_git::status(&ws_dir(sv))
}

/// 单文件 diff。
pub fn diff(sv: &Services, path: &str) -> Result<GitDiffVO> {
    wb_git::diff(&ws_dir(sv), path)
}

/// 提交历史。
pub fn history(sv: &Services, limit: i64) -> Result<Vec<GitCommitVO>> {
    wb_git::history(&ws_dir(sv), limit)
}

/// 单提交详情（path 非空附该文件 diff）。
pub fn commit_detail(sv: &Services, hash: &str, path: Option<&str>) -> Result<GitCommitDetailVO> {
    wb_git::commit_detail(&ws_dir(sv), hash, path)
}

/// 单文件历史。
pub fn file_history(sv: &Services, path: &str, limit: i64) -> Result<Vec<GitCommitVO>> {
    wb_git::file_history(&ws_dir(sv), path, limit)
}

/// 分支清单。
pub fn branches(sv: &Services) -> Result<Vec<GitBranchVO>> {
    wb_git::branches(&ws_dir(sv))
}

/// 暂存并回吐快照。
pub fn stage(sv: &Services, paths: &[String]) -> Result<GitStatusVO> {
    wb_git::write::stage(&ws_dir(sv), paths)?;
    Ok(status(sv))
}

/// 取消暂存并回吐快照。
pub fn unstage(sv: &Services, paths: &[String]) -> Result<GitStatusVO> {
    wb_git::write::unstage(&ws_dir(sv), paths)?;
    Ok(status(sv))
}

/// 丢弃工作区改动并回吐快照。
pub fn discard(sv: &Services, paths: &[String]) -> Result<GitStatusVO> {
    wb_git::write::discard(&ws_dir(sv), paths)?;
    Ok(status(sv))
}

/// 标记冲突已解决并回吐快照。
pub fn resolve(sv: &Services, paths: &[String]) -> Result<GitStatusVO> {
    wb_git::write::resolve(&ws_dir(sv), paths)?;
    Ok(status(sv))
}

/// 提交并回吐快照与新 hash。
pub fn commit(sv: &Services, message: &str, amend: bool) -> Result<GitCommitRESP> {
    let hash = wb_git::write::commit(&ws_dir(sv), message, amend)?;
    Ok(GitCommitRESP {
        resp_status: status(sv),
        resp_hash: hash,
    })
}

/// 新建分支并回吐快照。
pub fn branch_create(sv: &Services, name: &str) -> Result<GitStatusVO> {
    wb_git::write::branch_create(&ws_dir(sv), name)?;
    Ok(status(sv))
}

/// 切换分支并回吐快照。
pub fn switch_branch(sv: &Services, name: &str) -> Result<GitStatusVO> {
    wb_git::write::switch_branch(&ws_dir(sv), name)?;
    Ok(status(sv))
}

/// 删除已合并分支并回吐快照。
pub fn branch_delete(sv: &Services, name: &str) -> Result<GitStatusVO> {
    wb_git::write::branch_delete(&ws_dir(sv), name)?;
    Ok(status(sv))
}

/// 合并 / 变基；冲突时回吐带中间态的快照而不是报错。
pub fn merge(sv: &Services, name: &str, rebase: bool) -> Result<GitStatusVO> {
    let conflict = wb_git::write::merge_branch(&ws_dir(sv), name, rebase)?;
    let mut st = status(sv);
    if conflict {
        st.git_conflict = true;
    }
    Ok(st)
}

/// 中间态继续 / 中止并回吐快照。
pub fn state_action(sv: &Services, action: &str) -> Result<GitStatusVO> {
    wb_git::write::state_action(&ws_dir(sv), action)?;
    Ok(status(sv))
}

/// 拉取；冲突时回吐带中间态的快照而不是报错。
pub fn pull(sv: &Services, remote: &str, branch: &str, rebase: bool) -> Result<GitStatusVO> {
    let conflict = wb_git::write::pull(&ws_dir(sv), remote, branch, rebase)?;
    let mut st = status(sv);
    if conflict {
        st.git_conflict = true;
    }
    Ok(st)
}

/// 取回远端并回吐快照。
pub fn fetch(sv: &Services, remote: &str) -> Result<GitStatusVO> {
    wb_git::write::fetch(&ws_dir(sv), remote)?;
    Ok(status(sv))
}

/// 推送并回吐快照。
pub fn push(sv: &Services, req: &GitPushREQ) -> Result<GitStatusVO> {
    wb_git::write::push(&ws_dir(sv), req)?;
    Ok(status(sv))
}

/// 初始化仓库并回吐快照。
pub fn init(sv: &Services, branch: &str) -> Result<GitStatusVO> {
    wb_git::write::init(&ws_dir(sv), branch)?;
    Ok(status(sv))
}

/// 登记远端并回吐快照。
pub fn remote_add(sv: &Services, name: &str, url: &str) -> Result<GitStatusVO> {
    wb_git::write::remote_add(&ws_dir(sv), name, url)?;
    Ok(status(sv))
}

/// 提交信息起草：依赖内核 no-session 短轮询，v1 预留（走 chat 通道外接入后补）。
pub fn draft_message(sv: &Services, _paths: &[String]) -> Result<GitMessageRESP> {
    let _ = sv;
    Ok(GitMessageRESP {
        resp_message: String::new(),
        resp_err: "提交信息起草还未接入模型，请手写提交信息".into(),
    })
}
