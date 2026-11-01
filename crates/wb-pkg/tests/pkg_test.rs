//! wb-pkg 场景测试：路径安全边界。路径逃逸漏判会让助手读写工作区之外文件，是唯一单独守的边界。

use std::path::{Path, PathBuf};

use wb_pkg::realpath::{lexical_clean, real_path, relative_to};

#[test]
fn path_tools_reject_escape_and_normalize() {
    assert_eq!(
        lexical_clean(Path::new("D:\\a\\b\\..\\c")),
        PathBuf::from("D:\\a\\c")
    );

    let err = relative_to(Path::new("D:\\root"), Path::new("D:\\other\\x")).unwrap_err();
    assert_eq!(err.code, "1003", "越界必须报路径逃逸");

    let rel = relative_to(Path::new("D:\\root"), Path::new("D:\\root\\sub\\a.txt")).unwrap();
    assert_eq!(rel, PathBuf::from("sub").join("a.txt"));
    assert_eq!(
        relative_to(Path::new("D:\\root"), Path::new("D:\\root")).unwrap(),
        PathBuf::new()
    );

    let existing = std::env::temp_dir();
    let normalized = real_path(Path::new(&format!(r"\\?\{}", existing.display()))).unwrap();
    assert!(
        !normalized.to_string_lossy().starts_with("\\\\?"),
        "存在路径归一后不该带长路径前缀：{normalized:?}"
    );
    let missing = real_path(Path::new("D:\\definitely-not-here-xyz")).unwrap();
    assert!(
        missing
            .to_string_lossy()
            .contains("definitely-not-here-xyz"),
        "缺失路径应按词法清理原样返回：{missing:?}"
    );
}

// ---- 子进程纪律 ----

/// 只有「不启子进程就能判断」才允许在界面路径上探测：
/// 以前用 `where` 探测会闪控制台，界面上表现为「开一个菜单闪一下黑框」。
#[test]
fn command_resolution_never_spawns_a_process() {
    assert!(
        wb_pkg::proc::resolve_command("cmd").is_some(),
        "cmd 必然在 PATH 里"
    );
    assert!(
        wb_pkg::proc::resolve_command("wb-definitely-not-here-xyz").is_none(),
        "不存在的命令必须返回 None"
    );
    // npm 装的工具靠 .cmd 启动，用户不会带后缀；解析必须自己展开
    assert!(
        wb_pkg::proc::resolve_command("npm").is_some(),
        "npm 解析失败说明 .cmd 后缀没有展开"
    );
}
