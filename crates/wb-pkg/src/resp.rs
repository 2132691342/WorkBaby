//! Resp 信封：HTTP 统一出参 `Resp{code,message,data}`，code 为 "0" 即成功。

use serde::ser::SerializeStruct;
use serde::{Serialize, Serializer};

/// 统一响应信封：code 永远是字符串，数组字段永不产出 null（空 Vec 序列化为 []）。
#[derive(Debug, Clone)]
pub struct Resp<T: Serialize = serde_json::Value> {
    /// 业务码："0" 成功，其余见 docs/ERROR-CODES.md
    pub code: String,
    /// 面向用户的一句话
    pub message: String,
    /// 载荷
    pub data: T,
}

impl<T: Serialize> Resp<T> {
    /// 成功响应。
    pub fn ok(data: T) -> Self {
        Self {
            code: "0".into(),
            message: "ok".into(),
            data,
        }
    }

    /// 失败响应（由 AppError 转换；载荷恒为 null）。
    pub fn err(e: &crate::error::AppError) -> Resp<serde_json::Value> {
        Resp::<serde_json::Value> {
            code: e.code.clone(),
            message: e.message.clone(),
            data: serde_json::Value::Null,
        }
    }
}

impl<T: Serialize> Serialize for Resp<T> {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        let mut st = serializer.serialize_struct("Resp", 3)?;
        st.serialize_field("code", &self.code)?;
        st.serialize_field("message", &self.message)?;
        st.serialize_field("data", &self.data)?;
        st.end()
    }
}

/// 空 data 的成功响应。
pub fn ok_empty() -> Resp<serde_json::Value> {
    Resp::ok(serde_json::Value::Null)
}
