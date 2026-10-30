//! wb-pkg 场景测试：路径安全边界。
//! 路径逃逸一旦漏判，助手就能读写工作区之外的文件——这是唯一值得单独守的边界。

use std::path::{Path, PathBuf};

use wb_pkg::realpath::{lexical_clean, real_path, relative_to};

/// `..` 的词法归一：`D:\a\b\..\c` 应落在 `D:\a\c`，不触碰文件系统。
#[test]
fn dotdot_is_resolved_lexically() {
    assert_eq!(
        lexical_clean(Path::new("D:\\a\\b\\..\\c")),
        PathBuf::from("D:\\a\\c")
    );
}

/// 逃出允许根的路径必须被拒，且报 1003（路径越界）。
#[test]
fn path_outside_root_is_rejected() {
    let err = relative_to(Path::new("D:\\root"), Path::new("D:\\other\\x")).unwrap_err();
    assert_eq!(err.code, "1003");
}

/// 允许根内的路径归一为相对路径；根自身归一为空串。
#[test]
fn path_inside_root_becomes_relative() {
    let rel = relative_to(Path::new("D:\\root"), Path::new("D:\\root\\sub\\a.txt")).unwrap();
    assert_eq!(rel, PathBuf::from("sub").join("a.txt"));
    assert_eq!(
        relative_to(Path::new("D:\\root"), Path::new("D:\\root")).unwrap(),
        PathBuf::new()
    );
}

/// `real_path` 归一长路径前缀；不存在的路径不报错（交给上层判存在性）。
/// 存在路径经 canonicalize 解析，前缀被剥掉；缺失路径按词法清理原样返回。
#[test]
fn real_path_normalizes_prefix_and_tolerates_missing() {
    let existing = std::env::temp_dir();
    let prefixed = format!(r"\\?\{}", existing.display());
    let normalized = real_path(Path::new(&prefixed)).unwrap();
    assert!(
        !normalized.to_string_lossy().starts_with("\\\\?"),
        "存在路径归一后不该带长路径前缀：{normalized:?}"
    );

    let missing = real_path(Path::new("D:\\definitely-not-here-xyz")).unwrap();
    assert!(
        missing
            .to_string_lossy()
            .contains("definitely-not-here-xyz"),
        "缺失路径应原样返回：{missing:?}"
    );
}
