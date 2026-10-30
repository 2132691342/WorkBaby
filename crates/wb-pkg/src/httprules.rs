//! HTTP 纪律：GET/POST 白名单与路径风格校验（api 注册路由时调用）。

use crate::error::{AppError, Result};

/// 允许的 HTTP 方法。
pub const ALLOWED_METHODS: [&str; 2] = ["GET", "POST"];

/// 校验方法在白名单内；违规报错（开发期拦截，线上路由写死所以只防手滑）。
pub fn check_method(method: &str) -> Result<()> {
    if ALLOWED_METHODS.contains(&method) {
        Ok(())
    } else {
        Err(AppError::new("1001", "仅允许 GET 与 POST 方法").with_details(method.to_string()))
    }
}

/// 校验业务路径风格：`/api/v1/{resource}/{action}/:id`，id 只能是最后一段。
pub fn check_path(path: &str) -> Result<()> {
    if !path.starts_with("/api/v1/") {
        return Err(AppError::new("1001", "业务路径必须以 /api/v1/ 开头").with_details(path));
    }
    let rest = &path["/api/v1/".len()..];
    let segs: Vec<&str> = rest.split('/').filter(|s| !s.is_empty()).collect();
    if segs.is_empty() {
        return Err(AppError::new("1001", "路径缺少资源段").with_details(path.to_string()));
    }
    // 参数段（:id）只允许出现在末尾
    let params: Vec<usize> = segs
        .iter()
        .enumerate()
        .filter(|(_, s)| s.starts_with(':'))
        .map(|(i, _)| i)
        .collect();
    if let Some(last) = params.last() {
        if *last != segs.len() - 1 || params.len() > 1 {
            return Err(AppError::new("1001", "变量参数只能落在路径末尾且最多一个")
                .with_details(path.to_string()));
        }
    }
    Ok(())
}
