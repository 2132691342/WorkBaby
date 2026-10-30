//! AppError：业务错误的唯一形态，code 是数字字符串，与 HTTP 状态码在类型上分开。

use std::fmt;

/// 业务错误：code 段位见 docs/ERROR-CODES.md；message 面向用户，details 面向排障。
#[derive(Debug, Clone)]
pub struct AppError {
    /// 错误码（数字字符串，如 "1001"）
    pub code: String,
    /// 面向用户的一句话
    pub message: String,
    /// 面向排障的原文（stderr、io error 等）
    pub details: Option<String>,
}

pub type Result<T> = std::result::Result<T, AppError>;

impl AppError {
    /// 构造业务错误（无底层原因）。
    pub fn new(code: &str, message: impl Into<String>) -> Self {
        Self {
            code: code.to_string(),
            message: message.into(),
            details: None,
        }
    }

    /// 构造业务错误并附底层原因原文。
    pub fn wrap(code: &str, message: impl Into<String>, cause: impl fmt::Display) -> Self {
        Self {
            code: code.to_string(),
            message: message.into(),
            details: Some(cause.to_string()),
        }
    }

    /// 补充排障细节（链式）。
    pub fn with_details(mut self, details: impl Into<String>) -> Self {
        self.details = Some(details.into());
        self
    }
}

impl fmt::Display for AppError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match &self.details {
            Some(d) => write!(f, "[{}] {} ({})", self.code, self.message, d),
            None => write!(f, "[{}] {}", self.code, self.message),
        }
    }
}

impl std::error::Error for AppError {}
