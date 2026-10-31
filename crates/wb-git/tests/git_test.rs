//! wb-git 场景测试：porcelain 协议解析（快）+ 真 git 仓库全链路（慢，#[ignore]）。
//! 真仓库链路依赖系统 git，只在提交前显式跑：cargo test -p wb-git -- --ignored

use wb_git::parse::{classify, porcelain_z};

/// 状态解析两头验：-z 输出里中文与空格路径原样保留；
/// 归类把 MM 拆两条、未跟踪归 ?、UU 标冲突——错了界面就把同一文件显示成两行。
#[test]
fn status_parsing_keeps_unicode_and_classifies_rows() {
    let entries = porcelain_z("M  src/界 面.rs\0?? 未跟踪 文件.txt\0");
    assert_eq!(entries.len(), 2, "两条 NUL 分隔条目都应解析出来");
    assert_eq!(entries[0].path, "src/界 面.rs");
    assert_eq!(entries[1].path, "未跟踪 文件.txt");
    assert_eq!((entries[1].staged, entries[1].unstaged), ('?', '?'));

    let rows = classify(&porcelain_z(
        "MM a.txt\0M  b.txt\0 D c.txt\0UU d.txt\0?? e.txt\0",
    ));
    // MM→2 条；M 空格→1 条；空格 D→1 条；UU→1 条冲突；??→1 条
    assert_eq!(rows.len(), 6, "MM 双条 + 单条 4 = 6");
    let conflict = rows
        .iter()
        .find(|(_, _, _, cf)| *cf)
        .expect("UU 应标记冲突");
    assert_eq!(conflict.1, 'U');
    let untracked = rows
        .iter()
        .find(|(_, st, _, _)| *st == '?')
        .expect("?? 应归未跟踪");
    assert!(!untracked.2, "未跟踪不算已暂存");
}

/// 真 git 链路：建仓 → 写文件 → status → diff → 暂存 → 提交 → 历史 → 分支，
/// 外加两个拒绝边界：空提交信息、工作区外路径；不存在的文件 diff 报错不 panic。
#[test]
#[ignore]
fn full_repo_roundtrip_works() {
    let tmp = std::env::temp_dir().join(format!("wb-git-test-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&tmp);
    std::fs::create_dir_all(&tmp).unwrap();

    let status0 = wb_git::status(&tmp);
    assert!(!status0.git_is_repo, "空目录不应被当作仓库");

    wb_git::write::init(&tmp, "main").unwrap();
    let st = wb_git::status(&tmp);
    assert!(st.git_is_repo, "init 后应是仓库");
    assert_eq!(st.git_branch, "main");

    std::fs::write(tmp.join("界 面.txt"), "hello\nworkbaby\n").unwrap();
    let st = wb_git::status(&tmp);
    assert_eq!(st.git_files.len(), 1, "新文件应出现在状态里");
    assert_eq!(st.git_files[0].git_path, "界 面.txt");
    assert_eq!(st.git_files[0].git_status, "?");

    let diff = wb_git::diff(&tmp, "界 面.txt").unwrap();
    assert_eq!(diff.git_lines.len(), 2, "未跟踪文件整篇是新增行");
    assert!(
        wb_git::diff(&tmp, "不存在.txt").is_err(),
        "不存在的文件 diff 应报错而不是空结果"
    );

    wb_git::write::stage(&tmp, &["界 面.txt".into()]).unwrap();
    let st = wb_git::status(&tmp);
    assert!(st.git_files[0].git_staged, "add 之后应是已暂存");

    let hash = wb_git::write::commit(&tmp, "首提交：中文信息", false).unwrap();
    assert_eq!(hash.len(), 40, "应返回完整 hash");

    let hist = wb_git::history(&tmp, 10).unwrap();
    assert_eq!(hist.len(), 1, "一次提交一条历史");
    assert_eq!(hist[0].git_subject, "首提交：中文信息");
    assert_eq!(hist[0].git_files, 1);

    let branches = wb_git::branches(&tmp).unwrap();
    assert!(
        branches
            .iter()
            .any(|b| b.git_name == "main" && b.git_current),
        "main 应是当前分支"
    );

    // 空信息提交被拒
    assert!(
        wb_git::write::commit(&tmp, "  ", false).is_err(),
        "空提交信息必须被拒绝"
    );
    // 越界路径被拒
    let outside = std::env::temp_dir().display().to_string();
    assert!(
        wb_git::write::stage(&tmp, &[format!("{outside}/evil.txt")]).is_err(),
        "工作区外路径必须被拒绝"
    );

    let _ = std::fs::remove_dir_all(&tmp);
}
