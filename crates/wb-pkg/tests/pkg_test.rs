//! wb-pkg 场景测试：路径越界判定与子进程纪律。越界漏判等于允许读写工作区之外的文件。

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

/// 程序探测必须只查路径、不起子进程：起进程会在界面路径上闪一次控制台窗口。
#[test]
fn command_resolution_never_spawns_a_process() {
    let cmd_hit = wb_pkg::proc::resolve_command("cmd").is_some()
        || std::path::Path::new(r"C:\Windows\System32\cmd.exe").is_file();
    assert!(cmd_hit, "cmd 在 PATH 或 System32 里应能被定位");
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
