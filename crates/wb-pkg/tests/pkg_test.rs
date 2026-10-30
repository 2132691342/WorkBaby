//! wb-pkg 场景测试：路径安全是唯一值得单独守的边界——它挡住工作区逃逸。

use std::path::{Path, PathBuf};

use wb_pkg::realpath::{lexical_clean, relative_to};

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
