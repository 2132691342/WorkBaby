//! 客户端工厂：按解析结果选择 wire 实现。

use wb_pkg::{AppError, Result};

use crate::config::ResolvedProvider;
use crate::openai::OpenAiClient;
use crate::ModelClient;

/// 构造指定 provider 的客户端。
/// 未实现的 wire api 直接报错，不做静默降级——降级会让用户以为在用某种协议，
/// 实际发出去的是另一种格式。
pub fn create_client(p: ResolvedProvider) -> Result<std::sync::Arc<dyn ModelClient>> {
    match p.api.as_str() {
        "openai-completions" | "" => Ok(std::sync::Arc::new(OpenAiClient::new(p))),
        other => Err(AppError::new(
            "3005",
            format!("暂不支持的 wire api：{other}（当前支持 openai-completions）"),
        )),
    }
}
