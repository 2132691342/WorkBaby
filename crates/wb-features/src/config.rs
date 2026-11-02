//! 功能级配置：用户在功能页改的值，随 FeatureCtx 注入到功能代码。
//! 存储走设置 KV（`feature_config_{code}`），序列化为 JSON；缺省即默认值。

use serde::{Deserialize, Serialize};

/// web 功能配置。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(default, rename_all = "snake_case")]
pub struct WebConfig {
    /// 锁定的搜索源码；空 = 自动排队（有密钥的 API 源在前）
    pub web_provider: String,
    /// 单次搜索返回条数上限
    pub web_search_limit: usize,
    /// HTTP 代理；空 = 直连
    pub web_proxy: String,
}

impl Default for WebConfig {
    /// 默认：不锁源、8 条、直连。
    fn default() -> Self {
        Self {
            web_provider: String::new(),
            web_search_limit: 8,
            web_proxy: String::new(),
        }
    }
}

impl WebConfig {
    /// 收口成合法区间：条数 1..=20，源码必须是已知源。
    pub fn sanitized(mut self) -> Self {
        self.web_search_limit = self.web_search_limit.clamp(1, 20);
        self.web_provider = self.web_provider.trim().to_string();
        self.web_proxy = self.web_proxy.trim().to_string();
        self
    }
}

/// memory 功能配置。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(default, rename_all = "snake_case")]
pub struct MemoryConfig {
    /// 自动整合触发条数（低于 2 视为关闭自动整合）
    pub memory_auto_threshold: usize,
    /// 相似度合并阈值（0.1..=1.0）
    pub memory_merge_threshold: f64,
    /// 单次检索返回条数
    pub memory_search_limit: usize,
}

impl Default for MemoryConfig {
    /// 默认与既有常量一致：40 条触发、0.5 阈值、10 条返回。
    fn default() -> Self {
        Self {
            memory_auto_threshold: 40,
            memory_merge_threshold: 0.5,
            memory_search_limit: 10,
        }
    }
}

impl MemoryConfig {
    /// 收口成合法区间：阈值 0.1..=1.0，返回条数 1..=50。
    pub fn sanitized(mut self) -> Self {
        self.memory_merge_threshold = self.memory_merge_threshold.clamp(0.1, 1.0);
        self.memory_search_limit = self.memory_search_limit.clamp(1, 50);
        self
    }
}

/// lsp 功能配置。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "snake_case")]
pub struct LspConfig {
    /// 启用的服务器名清单；空 = 全部已安装的服务器都可用
    pub lsp_servers: Vec<String>,
}

impl LspConfig {
    /// 该服务器是否被允许（空清单 = 不限制）。
    pub fn allows(&self, server_name: &str) -> bool {
        self.lsp_servers.is_empty() || self.lsp_servers.iter().any(|n| n == server_name)
    }
}

/// 功能配置总表：按功能码取对应段。
#[derive(Debug, Clone, Default)]
pub struct FeatureConfigs {
    /// web 配置
    pub web: WebConfig,
    /// memory 配置
    pub memory: MemoryConfig,
    /// lsp 配置
    pub lsp: LspConfig,
}

impl FeatureConfigs {
    /// 从「码 → JSON 原文」的映射里解析；解析失败用默认值，不让坏配置挡住功能。
    pub fn parse(raw: impl Fn(&str) -> Option<String>) -> Self {
        Self {
            web: parse_json::<WebConfig>(raw("web"))
                .unwrap_or_default()
                .sanitized(),
            memory: parse_json::<MemoryConfig>(raw("memory"))
                .unwrap_or_default()
                .sanitized(),
            lsp: parse_json::<LspConfig>(raw("lsp")).unwrap_or_default(),
        }
    }
}

/// 单段 JSON 解析：空串与坏 JSON 都返回 None。
fn parse_json<T: for<'de> Deserialize<'de>>(raw: Option<String>) -> Option<T> {
    let text = raw?;
    if text.trim().is_empty() {
        return None;
    }
    serde_json::from_str(&text).ok()
}
