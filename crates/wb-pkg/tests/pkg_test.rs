//! wb-pkg 场景测试：路径安全边界。
//! 路径逃逸一旦漏判，助手就能读写工作区之外的文件——这是唯一值得单独守的边界。

use std::path::{Path, PathBuf};

use wb_pkg::realpath::{lexical_clean, real_path, relative_to};

/// 路径处理三件事一件都不能错：`..` 词法归一不触碰文件系统；
/// 逃出允许根的路径报 1003；根内归一为相对路径、根自身归一为空串；
/// `real_path` 剥掉长路径前缀且不存在的路径不报错（交给上层判存在性）。
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
