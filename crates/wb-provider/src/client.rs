//! 客户端工厂：按解析结果选择 wire 实现（v1 先落 openai-completions；
//! anthropic-messages 与 google-generative-ai 的实现进同一 trait，补齐时只加文件）。

use wb_pkg::{AppError, Result};

use crate::config::ResolvedProvider;
use crate::openai::OpenAiClient;
use crate::ModelClient;

/// 构造指定 provider 的客户端。
pub fn create_client(p: ResolvedProvider) -> Result<std::sync::Arc<dyn ModelClient>> {
    match p.api.as_str() {
        "openai-completions" | "" => Ok(std::sync::Arc::new(OpenAiClient::new(p))),
        other => Err(AppError::new(
            "3005",
            format!(
                "暂不支持的 wire api：{other}（规划内：anthropic-messages / google-generative-ai）"
            ),
        )),
    }
}
