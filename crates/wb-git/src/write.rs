//! 白名单写动作：动作集合封闭，拒绝面清晰（docs/WORKSPACE.md）。一律拒绝 reset --hard / clean -fd / 裸 --force。

use std::path::{Path, PathBuf};

use wb_domain::error::common;
use wb_domain::git::GitPushREQ;
use wb_pkg::{AppError, Result};

use crate::exec;
use crate::RepoCtx;

/// 校验并折算成仓库根相对路径；越界 / 空串报 1003 / 1001。
fn ensure_paths(ctx: &RepoCtx, paths: &[String]) -> Result<Vec<String>> {
    if paths.is_empty() {
        return Err(AppError::new(common::PARAM_INVALID, "没有给出文件路径"));
    }
    paths
        .iter()
        .map(|p| {
            let p = p.trim();
            if p.is_empty() || p.contains("..") {
                return Err(AppError::new(common::PATH_ESCAPE, format!("路径非法：{p}")));
            }
            Ok(ctx.root_rel(p))
        })
        .collect()
}

/// 暂存。
pub fn stage(ws: &Path, paths: &[String]) -> Result<()> {
    let ctx = RepoCtx::open(ws)?;
    let rel = ensure_paths(&ctx, paths)?;
    let mut args = vec!["add", "--"];
    args.extend(rel.iter().map(|s| s.as_str()));
    exec::ok(&ctx.root, &args).map(|_| ())
}

/// 取消暂存。
pub fn unstage(ws: &Path, paths: &[String]) -> Result<()> {
    let ctx = RepoCtx::open(ws)?;
    let rel = ensure_paths(&ctx, paths)?;
    let mut args = vec!["restore", "--staged", "--"];
    args.extend(rel.iter().map(|s| s.as_str()));
    exec::ok(&ctx.root, &args).map(|_| ())
}

/// 丢弃工作区改动：未跟踪文件删除，其余 restore 工作区。
pub fn discard(ws: &Path, paths: &[String]) -> Result<()> {
    let ctx = RepoCtx::open(ws)?;
    let rel = ensure_paths(&ctx, paths)?;
    for p in &rel {
        let tracked = !exec::run(&ctx.root, &["ls-files", "--", p])
            .stdout
            .trim()
            .is_empty();
        if tracked {
            exec::ok(&ctx.root, &["restore", "--", p])?;
        } else {
            let full = ctx.root.join(p.replace('/', "\\"));
            if full.is_file() {
                std::fs::remove_file(&full)
                    .map_err(|e| AppError::wrap(common::IO_FAILED, "删除未跟踪文件失败", e))?;
            }
        }
    }
    Ok(())
}

/// 标记冲突已解决（仅合并 / 变基中间态可用）。
pub fn resolve(ws: &Path, paths: &[String]) -> Result<()> {
    let ctx = RepoCtx::open(ws)?;
    let (state, _) = crate::intermediate_state(&ctx.root);
    if state == "none" {
        return Err(AppError::new(
            common::GIT_CONFLICT,
            "当前没有进行中的合并或变基",
        ));
    }
    let rel = ensure_paths(&ctx, paths)?;
    let mut args = vec!["add", "--"];
    args.extend(rel.iter().map(|s| s.as_str()));
    exec::ok(&ctx.root, &args).map(|_| ())
}

/// 提交（信息非空；--amend 仅上游未含 HEAD 时允许）。返回新提交 hash。
pub fn commit(ws: &Path, message: &str, amend: bool) -> Result<String> {
    let ctx = RepoCtx::open(ws)?;
    if message.trim().is_empty() {
        return Err(AppError::new(common::GIT_COMMIT_FAILED, "提交信息不能为空"));
    }
    if amend && pushed_ahead(&ctx.root) {
        return Err(AppError::new(
            common::GIT_COMMIT_FAILED,
            "该提交已推送到远端，不能 amend",
        ));
    }
    let msg_file = write_msg_file(&ctx.root, message)?;
    let mut args = vec!["commit", "-F", &msg_file];
    if amend {
        args.push("--amend");
    }
    let r = exec::ok(&ctx.root, &args);
    let _ = std::fs::remove_file(ctx.root.join(&msg_file));
    r?;
    Ok(exec::ok(&ctx.root, &["rev-parse", "HEAD"])?
        .trim()
        .to_string())
}

/// HEAD 是否已在上游里（被推送过）。
fn pushed_ahead(root: &Path) -> bool {
    let upstream = exec::run(root, &["rev-parse", "--abbrev-ref", "@{u}"])
        .stdout
        .trim()
        .to_string();
    if upstream.is_empty() {
        return false;
    }
    // 上游是 HEAD 的祖先 = 本地领先 = 未推送；否则已推送
    exec::run(root, &["merge-base", "--is-ancestor", "@{u}", "HEAD"]).git_exit_code != 0
}

/// 提交信息落临时文件（避免命令行转义与长度限制）。
fn write_msg_file(root: &Path, message: &str) -> Result<String> {
    let path = root.join(".git").join("WB_COMMIT_MSG");
    std::fs::write(&path, message)
        .map_err(|e| AppError::wrap(common::IO_FAILED, "写入提交信息文件失败", e))?;
    Ok(".git/WB_COMMIT_MSG".to_string())
}

/// 分支名合法性：禁空白 / .. / 控制字符 / ~^:?*[\ / 开头 - / 结尾 .。
fn ensure_branch_name(name: &str) -> Result<()> {
    let bad = name.is_empty()
        || name.starts_with('-')
        || name.ends_with('/')
        || name.ends_with('.')
        || name.contains("..")
        || name.contains([' ', '~', '^', ':', '?', '*', '[', '\\'])
        || name.chars().any(|c| c.is_control());
    if bad {
        return Err(AppError::new(
            common::PARAM_INVALID,
            format!("分支名非法：{name}"),
        ));
    }
    Ok(())
}

/// 新建分支。
pub fn branch_create(ws: &Path, name: &str) -> Result<()> {
    ensure_branch_name(name)?;
    let ctx = RepoCtx::open(ws)?;
    exec::ok(&ctx.root, &["branch", name]).map(|_| ())
}

/// 删除已合并分支（拒绝 -D）。
pub fn branch_delete(ws: &Path, name: &str) -> Result<()> {
    ensure_branch_name(name)?;
    let ctx = RepoCtx::open(ws)?;
    exec::ok(&ctx.root, &["branch", "-d", name]).map(|_| ())
}

/// 切换分支（脏工作区由 git 拒绝，报错透出给界面确认）。
pub fn switch_branch(ws: &Path, name: &str) -> Result<()> {
    ensure_branch_name(name)?;
    let ctx = RepoCtx::open(ws)?;
    exec::ok(&ctx.root, &["switch", name]).map(|_| ())
}

/// 合并 / 变基；产生冲突中间态时返回 true（界面横幅处理）。
pub fn merge_branch(ws: &Path, name: &str, rebase: bool) -> Result<bool> {
    ensure_branch_name(name)?;
    let ctx = RepoCtx::open(ws)?;
    let out = if rebase {
        exec::run(&ctx.root, &["rebase", name])
    } else {
        exec::run(&ctx.root, &["merge", "--no-edit", name])
    };
    if out.ok() {
        return Ok(false);
    }
    let (state, _) = crate::intermediate_state(&ctx.root);
    if state == "merge" || state == "rebase" {
        return Ok(true);
    }
    Err(AppError::new(
        common::GIT_FAILED,
        out.stderr.lines().next().unwrap_or("git 操作失败"),
    ))
}

/// 中间态继续 / 中止。
pub fn state_action(ws: &Path, action: &str) -> Result<()> {
    let ctx = RepoCtx::open(ws)?;
    let (state, _) = crate::intermediate_state(&ctx.root);
    match (action, state.as_str()) {
        ("continue", "merge") => exec::ok(&ctx.root, &["merge", "--continue"]).map(|_| ()),
        ("continue", "rebase") => exec::ok(&ctx.root, &["rebase", "--continue"]).map(|_| ()),
        ("abort", "merge") => exec::ok(&ctx.root, &["merge", "--abort"]).map(|_| ()),
        ("abort", "rebase") => exec::ok(&ctx.root, &["rebase", "--abort"]).map(|_| ()),
        _ => Err(AppError::new(
            common::GIT_CONFLICT,
            "当前没有可继续或中止的合并 / 变基",
        )),
    }
}

/// 拉取：默认 --ff-only 防误合并；冲突中间态返回 true；分叉报 1024。
pub fn pull(ws: &Path, remote: &str, branch: &str, rebase: bool) -> Result<bool> {
    let ctx = RepoCtx::open(ws)?;
    let mut args = vec!["pull"];
    args.push(if rebase { "--rebase" } else { "--ff-only" });
    if !remote.is_empty() {
        args.push(remote);
        if !branch.is_empty() {
            args.push(branch);
        }
    }
    let out = exec::run(&ctx.root, &args);
    if out.ok() {
        return Ok(false);
    }
    let (state, _) = crate::intermediate_state(&ctx.root);
    if state == "merge" || state == "rebase" {
        return Ok(true);
    }
    if out.stderr.contains("Not possible to fast-forward") || out.stderr.contains("divergent") {
        return Err(AppError::new(
            common::GIT_DIVERGED,
            "远端与本地分叉，请显式合并或变基",
        ));
    }
    Err(AppError::new(
        common::GIT_FAILED,
        out.stderr.lines().next().unwrap_or("git pull 失败"),
    ))
}

/// 取回远端状态（不改工作区与本地分支）。
pub fn fetch(ws: &Path, remote: &str) -> Result<()> {
    let ctx = RepoCtx::open(ws)?;
    if remote.is_empty() {
        exec::ok(&ctx.root, &["fetch"]).map(|_| ())
    } else {
        exec::ok(&ctx.root, &["fetch", remote]).map(|_| ())
    }
}

/// 推送：强制级别逐级升级；force 必须与分支名一致的 confirm。
pub fn push(ws: &Path, req: &GitPushREQ) -> Result<()> {
    let ctx = RepoCtx::open(ws)?;
    let branch = if req.git_branch.is_empty() {
        exec::ok(&ctx.root, &["rev-parse", "--abbrev-ref", "HEAD"])?
            .trim()
            .to_string()
    } else {
        req.git_branch.clone()
    };
    ensure_branch_name(&branch)?;
    let remote = if req.git_remote.is_empty() {
        "origin".to_string()
    } else {
        req.git_remote.clone()
    };
    let mut args: Vec<String> = vec!["push".into()];
    if req.git_set_upstream {
        args.push("-u".into());
    }
    match req.git_force.as_str() {
        "lease" => args.push("--force-with-lease".into()),
        "force" => {
            if req.git_confirm != branch {
                return Err(AppError::new(
                    common::PARAM_INVALID,
                    "强制推送需要确认串与分支名完全一致",
                ));
            }
            args.push("--force".into());
        }
        _ => {}
    }
    args.push(remote);
    args.push(branch);
    let arg_refs: Vec<&str> = args.iter().map(|s| s.as_str()).collect();
    let out = exec::run(&ctx.root, &arg_refs);
    if out.ok() {
        return Ok(());
    }
    if out.stderr.contains("[rejected]") {
        return Err(AppError::new(
            common::GIT_DIVERGED,
            "推送被拒绝：先取回远端并对账",
        ));
    }
    Err(AppError::new(
        common::GIT_FAILED,
        out.stderr.lines().next().unwrap_or("git push 失败"),
    ))
}

/// 在工作区初始化仓库。
pub fn init(ws: &Path, branch: &str) -> Result<()> {
    if !branch.is_empty() {
        ensure_branch_name(branch)?;
        exec::ok(ws, &["init", "-b", branch]).map(|_| ())
    } else {
        exec::ok(ws, &["init"]).map(|_| ())
    }
}

/// 登记远端。
pub fn remote_add(ws: &Path, name: &str, url: &str) -> Result<()> {
    if name.is_empty() || url.is_empty() || name.contains(' ') {
        return Err(AppError::new(common::PARAM_INVALID, "远端名与地址不能为空"));
    }
    let ctx = RepoCtx::open(ws)?;
    exec::ok(&ctx.root, &["remote", "add", name, url]).map(|_| ())
}

/// 仓库根的绝对路径（路由层组装快照用）。
pub fn repo_root(ws: &Path) -> Result<PathBuf> {
    RepoCtx::open(ws).map(|c| c.root)
}
