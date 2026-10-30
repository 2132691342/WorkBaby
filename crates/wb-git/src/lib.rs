//! 工作区版本视图（读）：status / diff / 历史 / 分支。写动作见 write.rs，全部走系统 git。

pub mod exec;
pub mod parse;
pub mod write;

use std::path::{Path, PathBuf};

use wb_domain::error::common;
use wb_domain::git::{
    GitBranchVO, GitCommitDetailVO, GitCommitVO, GitDiffVO, GitFileVO, GitRemoteVO, GitStatusVO,
};
use wb_pkg::{AppError, Result};

/// diff 展示的元信息行（git show / log 的标题区不进 diff 行）。
pub struct RepoCtx {
    /// 仓库根
    pub root: PathBuf,
    /// 工作区相对仓库根的前缀（工作区即根时为空）
    pub ws_prefix: String,
}

impl RepoCtx {
    /// 打开仓库上下文；工作区不是仓库时报 1017。
    pub fn open(ws: &Path) -> Result<Self> {
        let root = exec::ok(ws, &["rev-parse", "--show-toplevel"])?;
        let root = PathBuf::from(root.trim());
        let ws_canon = wb_pkg::realpath::real_path(ws)?;
        let ws_prefix = if ws_canon == root {
            String::new()
        } else {
            let rel = ws_canon
                .strip_prefix(&root)
                .map_err(|_| AppError::new(common::PATH_ESCAPE, "路径越界：工作区不在仓库内"))?;
            let mut s = rel.to_string_lossy().into_owned();
            if !s.is_empty() && !s.ends_with(['/', '\\']) {
                s.push('/');
            }
            s.replace('\\', "/")
        };
        Ok(Self { root, ws_prefix })
    }

    /// 工作区相对路径 → 仓库根相对路径。
    pub fn root_rel(&self, ws_rel: &str) -> String {
        format!("{}{}", self.ws_prefix, ws_rel.replace('\\', "/"))
    }

    /// 仓库根相对路径 → 工作区相对路径；不属于本工作区返回 None。
    pub fn ws_rel(&self, root_rel: &str) -> Option<String> {
        let p = root_rel.replace('\\', "/");
        if self.ws_prefix.is_empty() {
            return Some(p);
        }
        p.strip_prefix(&self.ws_prefix).map(|s| s.to_string())
    }
}

/// 仓库状态快照（永不失败：git 缺失 / 非仓库以 available / is_repo + reason 表达）。
pub fn status(ws: &Path) -> GitStatusVO {
    let version = exec::version().unwrap_or_default();
    if version.is_empty() {
        return GitStatusVO::unavailable(
            &ws.display().to_string(),
            "未检测到 git，请先安装并加入 PATH".into(),
            version,
        );
    }
    let ctx = match RepoCtx::open(ws) {
        Ok(c) => c,
        Err(e) => return GitStatusVO::unavailable(&ws.display().to_string(), e.message, version),
    };
    let root = ctx.root.clone();
    let workspace = ws.display().to_string();
    let mut vo = GitStatusVO::unavailable(&workspace, String::new(), version);
    vo.git_available = true;
    vo.git_is_repo = true;
    vo.git_repo_root = root.display().to_string();

    // 无提交的新仓库 abbrev-ref 返回 "HEAD"；symbolic-ref 才能拿到未出生分支名
    vo.git_branch = {
        let sym = exec::run(&root, &["symbolic-ref", "--short", "-q", "HEAD"]);
        if sym.ok() {
            sym.stdout.trim().to_string()
        } else {
            "HEAD".to_string()
        }
    };
    vo.git_upstream = exec::run(
        &root,
        &["rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"],
    )
    .stdout
    .trim()
    .to_string();
    if !vo.git_upstream.is_empty() {
        let counts = exec::run(
            &root,
            &[
                "rev-list",
                "--left-right",
                "--count",
                &format!("{}...HEAD", vo.git_upstream),
            ],
        );
        let nums: Vec<i64> = counts
            .stdout
            .split_whitespace()
            .filter_map(|s| s.parse().ok())
            .collect();
        if nums.len() == 2 {
            vo.git_behind = nums[0];
            vo.git_ahead = nums[1];
        }
    }

    vo.git_remotes = remotes(&root);
    let (files, conflict) = changed_files(&ctx);
    vo.git_files = files;
    vo.git_conflict = conflict;
    let (state, detail) = intermediate_state(&root);
    vo.git_state = state;
    vo.git_state_detail = detail;
    vo
}

/// 远端清单（fetch 地址，按名去重）。
fn remotes(root: &Path) -> Vec<GitRemoteVO> {
    let out = exec::run(root, &["remote", "-v"]);
    let mut seen = std::collections::BTreeMap::new();
    for line in out.stdout.lines() {
        let mut it = line.split_whitespace();
        if let (Some(name), Some(url)) = (it.next(), it.next()) {
            seen.entry(name.to_string())
                .or_insert_with(|| url.to_string());
        }
    }
    seen.into_iter()
        .map(|(n, u)| GitRemoteVO {
            git_remote_name: n,
            git_remote_url: u,
        })
        .collect()
}

/// 变更文件清单：porcelain 分类 + 两路 numstat 读数。
fn changed_files(ctx: &RepoCtx) -> (Vec<GitFileVO>, bool) {
    let raw = exec::run(&ctx.root, &["status", "--porcelain=v1", "-z"]);
    let entries = parse::porcelain_z(&raw.stdout);
    let staged_num =
        parse::numstat(&exec::run(&ctx.root, &["diff", "--cached", "--numstat"]).stdout);
    let unstaged_num = parse::numstat(&exec::run(&ctx.root, &["diff", "--numstat"]).stdout);
    let mut files = Vec::new();
    let mut conflict = false;
    for (path, st, staged, is_conflict) in parse::classify(&entries) {
        let Some(ws_path) = ctx.ws_rel(&path) else {
            continue;
        };
        if is_conflict {
            conflict = true;
        }
        let num = if staged { &staged_num } else { &unstaged_num };
        let stat = num
            .get(&path.replace('\\', "/"))
            .copied()
            .unwrap_or_default();
        files.push(GitFileVO {
            git_path: ws_path,
            git_status: st.to_string(),
            git_staged: staged,
            git_conflict: is_conflict,
            git_additions: stat.additions,
            git_deletions: stat.deletions,
        });
    }
    (files, conflict)
}

/// 中间态判定：读 .git 下的哨兵文件。
pub(crate) fn intermediate_state(root: &Path) -> (String, String) {
    let git_dir = exec::run(root, &["rev-parse", "--git-path", "."]).stdout;
    let git_dir = PathBuf::from(git_dir.trim());
    let has = |name: &str| {
        let p = if git_dir.is_absolute() {
            git_dir.join(name)
        } else {
            root.join(&git_dir).join(name)
        };
        p.exists()
    };
    if has("MERGE_HEAD") {
        ("merge".into(), "合并进行中，解决冲突后可继续或中止".into())
    } else if has("rebase-merge") || has("rebase-apply") {
        ("rebase".into(), "变基进行中，解决冲突后可继续或中止".into())
    } else if has("CHERRY_PICK_HEAD") {
        ("cherry-pick".into(), "拣选进行中".into())
    } else if has("REVERT_HEAD") {
        ("revert".into(), "回退进行中".into())
    } else {
        ("none".into(), String::new())
    }
}

/// 单文件 diff：未跟踪整篇新增；已跟踪取 HEAD 对比；两者都不是报 1004。
pub fn diff(ws: &Path, path: &str) -> Result<GitDiffVO> {
    let ctx = RepoCtx::open(ws)?;
    let root_rel = ctx.root_rel(path);
    let tracked = !exec::run(&ctx.root, &["ls-files", "--", &root_rel])
        .stdout
        .trim()
        .is_empty();
    let full = ctx.root.join(&root_rel);
    if !tracked && !full.exists() {
        return Err(AppError::new(
            common::PATH_MISSING,
            format!("文件不存在：{path}"),
        ));
    }
    let raw = if tracked {
        exec::ok(&ctx.root, &["diff", "HEAD", "--no-color", "--", &root_rel])?
    } else {
        read_untracked(&full)
    };
    let (lines, truncated, dropped) = parse::diff_lines(&raw);
    Ok(GitDiffVO {
        git_diff_path: path.to_string(),
        git_truncated: truncated,
        git_dropped: dropped,
        git_lines: lines,
    })
}

/// 未跟踪文件整篇视为新增行。
fn read_untracked(p: &Path) -> String {
    match std::fs::read_to_string(p) {
        Ok(text) => text
            .lines()
            .map(|l| format!("+{l}"))
            .collect::<Vec<_>>()
            .join("\n"),
        Err(_) => String::new(),
    }
}

/// 提交历史（带每条增删读数）。
pub fn history(ws: &Path, limit: i64) -> Result<Vec<GitCommitVO>> {
    let ctx = RepoCtx::open(ws)?;
    let limit = limit.clamp(1, 500);
    // 记录分隔符放 format 开头：numstat 跟在头字段同一记录里，split 后首段为空
    let fmt = "%x1e%H%x1f%h%x1f%s%x1f%an%x1f%ct";
    let out = exec::ok(
        &ctx.root,
        &[
            "log",
            "-n",
            &limit.to_string(),
            &format!("--pretty=format:{fmt}"),
            "--numstat",
        ],
    )?;
    Ok(parse::log_commits(&out))
}

/// 单提交详情；path 非空时附该文件 diff。
pub fn commit_detail(ws: &Path, hash: &str, path: Option<&str>) -> Result<GitCommitDetailVO> {
    let ctx = RepoCtx::open(ws)?;
    ensure_hash(hash)?;
    let fmt = "%H%x1f%h%x1f%s%x1f%b%x1f%an%x1f%ct%x1f%P";
    let head = exec::ok(
        &ctx.root,
        &[
            "show",
            "-s",
            "--no-color",
            &format!("--pretty=format:{fmt}"),
            hash,
        ],
    )?;
    let f: Vec<&str> = head.split('\x1f').collect();
    if f.len() < 7 {
        return Err(AppError::new(common::GIT_FAILED, "提交信息解析失败"));
    }
    let stat = exec::ok(
        &ctx.root,
        &[
            "show",
            "--first-parent",
            "-m",
            "--no-color",
            "--numstat",
            "--pretty=format:",
            hash,
        ],
    )?;
    let names = exec::ok(
        &ctx.root,
        &[
            "show",
            "--first-parent",
            "-m",
            "--no-color",
            "--name-status",
            "--pretty=format:",
            hash,
        ],
    )?;
    let nums = parse::numstat(&stat);
    let mut files = Vec::new();
    for line in names.lines() {
        let mut it = line.splitn(2, '\t');
        let (Some(st), Some(p)) = (it.next(), it.next()) else {
            continue;
        };
        let Some(ws_path) = ctx.ws_rel(p.trim()) else {
            continue;
        };
        let n = nums.get(p.trim()).copied().unwrap_or_default();
        files.push(GitFileVO {
            git_path: ws_path,
            git_status: st.to_string(),
            git_staged: true,
            git_conflict: false,
            git_additions: n.additions,
            git_deletions: n.deletions,
        });
    }
    let git_diff = match path {
        Some(p) => {
            let raw = exec::ok(
                &ctx.root,
                &[
                    "show",
                    "--first-parent",
                    "-m",
                    "--no-color",
                    hash,
                    "--",
                    &ctx.root_rel(p),
                ],
            )?;
            Some(GitDiffVO {
                git_diff_path: p.to_string(),
                git_truncated: false,
                git_dropped: 0,
                git_lines: parse::diff_lines(&raw).0,
            })
        }
        None => None,
    };
    Ok(GitCommitDetailVO {
        git_hash: f[0].to_string(),
        git_short: f[1].to_string(),
        git_subject: f[2].to_string(),
        git_body: f[3].to_string(),
        git_author: f[4].to_string(),
        git_when: f[5].parse::<i64>().unwrap_or(0) * 1000,
        git_parents: f[6].split_whitespace().map(|s| s.to_string()).collect(),
        git_files: files,
        git_diff,
    })
}

/// hash 基本形态校验（十六进制 4..64 位）。
fn ensure_hash(hash: &str) -> Result<()> {
    let ok = !hash.is_empty() && hash.len() <= 64 && hash.chars().all(|c| c.is_ascii_hexdigit());
    if ok {
        Ok(())
    } else {
        Err(AppError::new(common::PARAM_INVALID, "提交 hash 非法"))
    }
}

/// 单文件历史（--follow 跟踪改名）。
pub fn file_history(ws: &Path, path: &str, limit: i64) -> Result<Vec<GitCommitVO>> {
    let ctx = RepoCtx::open(ws)?;
    let limit = limit.clamp(1, 500);
    let fmt = "%x1e%H%x1f%h%x1f%s%x1f%an%x1f%ct";
    let out = exec::ok(
        &ctx.root,
        &[
            "log",
            "-n",
            &limit.to_string(),
            "--follow",
            &format!("--pretty=format:{fmt}"),
            "--numstat",
            "--",
            &ctx.root_rel(path),
        ],
    )?;
    Ok(parse::log_commits(&out))
}

/// 本地与远端分支；本地分支带领先落后读数。
pub fn branches(ws: &Path) -> Result<Vec<GitBranchVO>> {
    let ctx = RepoCtx::open(ws)?;
    let mut out = Vec::new();
    let local = exec::ok(
        &ctx.root,
        &[
            "for-each-ref",
            "refs/heads",
            "--format=%(refname:short)%09%(upstream:short)%09%(HEAD)",
        ],
    )?;
    for line in local.lines() {
        let parts: Vec<&str> = line.split('\t').collect();
        if parts.is_empty() || parts[0].is_empty() {
            continue;
        }
        let name = parts[0].to_string();
        let upstream = parts.get(1).copied().unwrap_or("").to_string();
        let current = parts.get(2).copied().unwrap_or("").trim() == "*";
        let (ahead, behind) = branch_counts(&ctx.root, &upstream, &name);
        out.push(GitBranchVO {
            git_name: name,
            git_current: current,
            git_remote: String::new(),
            git_ahead: ahead,
            git_behind: behind,
        });
    }
    let remote = exec::ok(
        &ctx.root,
        &[
            "for-each-ref",
            "refs/remotes",
            "--format=%(refname:short)%09%(HEAD)",
        ],
    )?;
    for line in remote.lines() {
        let parts: Vec<&str> = line.split('\t').collect();
        let Some(name) = parts.first().copied() else {
            continue;
        };
        if name.ends_with("/HEAD") {
            continue;
        }
        let remote_name = name.split('/').next().unwrap_or("").to_string();
        out.push(GitBranchVO {
            git_name: name.to_string(),
            git_current: false,
            git_remote: remote_name,
            git_ahead: 0,
            git_behind: 0,
        });
    }
    Ok(out)
}

/// 分支对上游的领先落后（无上游 / 读数失败均为 0）。
fn branch_counts(root: &Path, upstream: &str, branch: &str) -> (i64, i64) {
    if upstream.is_empty() {
        return (0, 0);
    }
    let spec = format!("{upstream}...{branch}");
    let counts = exec::run(root, &["rev-list", "--left-right", "--count", &spec]);
    let nums: Vec<i64> = counts
        .stdout
        .split_whitespace()
        .filter_map(|s| s.parse().ok())
        .collect();
    if nums.len() == 2 {
        (nums[1], nums[0])
    } else {
        (0, 0)
    }
}
