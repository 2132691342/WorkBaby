//! ULID 生成：`{PREFIX}_{26位ULID}`，排序即时间序。

use ulid::Ulid;

/// 业务前缀：声明 id 的语义与归属（AGENTS.md §2.7）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Prefix {
    /// 会话
    Sess,
    /// 会话条目（JSONL 行）
    Ent,
    /// 助手消息
    Msg,
    /// 一轮运行
    Run,
    /// 工具调用
    Tc,
    /// 内核提问（chat:ask 桥）
    Ask,
}

impl Prefix {
    /// 前缀字符串形式。
    pub fn as_str(self) -> &'static str {
        match self {
            Prefix::Sess => "SESS",
            Prefix::Ent => "ENT",
            Prefix::Msg => "MSG",
            Prefix::Run => "RUN",
            Prefix::Tc => "TC",
            Prefix::Ask => "ASK",
        }
    }
}

/// 生成 `{PREFIX}_{26位ULID}` 形式的 id。
pub fn new_id(prefix: Prefix) -> String {
    format!("{}_{}", prefix.as_str(), Ulid::new())
}

/// 生成裸 26 位 ULID（自带业务前缀的实体自拼，如 GOAL_）。
pub fn raw_ulid() -> String {
    Ulid::new().to_string()
}

/// 校验 id 形如 `{合法前缀}_{26位ULID}`。
pub fn is_valid(id: &str) -> bool {
    let Some((prefix, body)) = id.split_once('_') else {
        return false;
    };
    let ok_prefix = ["SESS", "ENT", "MSG", "RUN", "TC", "ASK"].contains(&prefix);
    ok_prefix && body.len() == 26 && body.chars().all(|c| c.is_ascii_alphanumeric())
}

/// 取 id 的前缀段；不合法返回空串。
pub fn prefix_of(id: &str) -> &str {
    id.split('_').next().unwrap_or("")
}
